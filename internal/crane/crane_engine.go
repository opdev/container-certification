package crane

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/redhat-openshift-ecosystem/openshift-preflight/artifacts"
	"github.com/redhat-openshift-ecosystem/openshift-preflight/x/log"
	"github.com/redhat-openshift-ecosystem/openshift-preflight/x/plugin/v0"

	"github.com/opdev/container-certification/internal/authn"
	"github.com/opdev/container-certification/internal/defaults"
	"github.com/opdev/container-certification/internal/pyxis"
	"github.com/opdev/container-certification/internal/rpm"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	cranev1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/cache"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// CraneEngine implements a certification.CheckEngine, and leverage crane to interact with
// the container registry and target image.
type CraneEngine struct {
	// // Kubeconfig is a byte slice containing a valid Kubeconfig to be used by checks.
	// Kubeconfig []byte
	// DockerConfig is the credential required to pull the image.
	DockerConfig string
	// Image is what is being tested, and should contain the
	// fully addressable path (including registry, namespaces, etc)
	// to the image
	Image string
	// Checks is an array of all checks to be executed against
	// the image provided.
	Checks []plugin.Check // TODO: This probably needs to be local to this project, and not the pluggable codebase.

	// Platform is the container platform to use. E.g. amd64.
	Platform string

	// // IsBundle is an indicator that the asset is a bundle.
	// IsBundle bool

	// IsScratch is an indicator that the asset is a scratch image
	IsScratch bool

	// Insecure controls whether to allow an insecure connection to
	// the registry crane connects with.
	Insecure bool

	imageRef plugin.ImageReference
	results  plugin.Results
}

func export(img cranev1.Image, w io.Writer) error {
	fs := mutate.Extract(img)
	_, err := io.Copy(w, fs)
	return err
}

func (c *CraneEngine) ExecuteChecks(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info("target image", "image", c.Image)

	// prepare crane runtime options, if necessary
	options := []crane.Option{
		crane.WithContext(ctx),
		crane.WithAuthFromKeychain(
			authn.PreflightKeychain(
				ctx,
				// We configure the Preflight Keychain here.
				// In theory, we should not require further configuration
				// downstream because the PreflightKeychain is a singleton.
				// However, as long as we pass this same DockerConfig
				// value downstream, it shouldn't matter if the
				// keychain is reconfigured downstream.
				authn.WithDockerConfig(c.DockerConfig),
			),
		),
		crane.WithPlatform(&cranev1.Platform{
			OS:           "linux",
			Architecture: c.Platform,
		}),
		retryOnceAfter(5 * time.Second),
	}

	if c.Insecure {
		// Adding WithTransport opt is a workaround to allow for access to HTTPS
		// container registries with self-signed or non-trusted certificates.
		//
		// See https://github.com/google/go-containerregistry/issues/1553 for more context. If this issue
		// is resolved, then this workaround can likely be removed or adjusted to use new features in the
		// go-containerregistry project.
		rt := remote.DefaultTransport.(*http.Transport).Clone()
		rt.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint: gosec
		}

		options = append(options, crane.Insecure, crane.WithTransport(rt))
	}

	// pull the image and save to fs
	logger.V(log.DBG).Info("pulling image from target registry")
	img, err := crane.Pull(c.Image, options...)
	if err != nil {
		return fmt.Errorf("failed to pull remote container: %v", err)
	}

	// create tmpdir to receive extracted fs
	tmpdir, err := os.MkdirTemp(os.TempDir(), "preflight-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %v", err)
	}
	logger.V(log.DBG).Info("created temporary directory", "path", tmpdir)
	defer func() {
		if err := os.RemoveAll(tmpdir); err != nil {
			logger.Error(err, "unable to clean up tmpdir", "tempDir", tmpdir)
		}
	}()

	imageTarPath := path.Join(tmpdir, "cache")
	if err := os.Mkdir(imageTarPath, 0o755); err != nil {
		return fmt.Errorf("failed to create cache directory: %s: %v", imageTarPath, err)
	}

	img = cache.Image(img, cache.NewFilesystemCache(imageTarPath))

	containerFSPath := path.Join(tmpdir, "fs")
	if err := os.Mkdir(containerFSPath, 0o755); err != nil {
		return fmt.Errorf("failed to create container expansion directory: %s: %v", containerFSPath, err)
	}

	// export/flatten, and extract
	logger.V(log.DBG).Info("exporting and flattening image")
	r, w := io.Pipe()
	go func() {
		logger.V(log.DBG).Info("writing container filesystem", "outputDirectory", containerFSPath)

		// Close the writer with any errors encountered during
		// extraction. These errors will be returned by the reader end
		// on subsequent reads. If err == nil, the reader will return
		// EOF.
		w.CloseWithError(export(img, w))
	}()

	logger.V(log.DBG).Info("extracting container filesystem", "path", containerFSPath)
	if err := untar(ctx, containerFSPath, r); err != nil {
		return fmt.Errorf("failed to extract tarball: %v", err)
	}

	// explicitly discarding from the reader for cases where there is data in the reader after it sends an EOF
	_, err = io.Copy(io.Discard, r)
	if err != nil {
		return fmt.Errorf("failed to drain io reader: %v", err)
	}

	reference, err := name.ParseReference(c.Image)
	if err != nil {
		return fmt.Errorf("image uri could not be parsed: %v", err)
	}

	// store the image internals in the engine image reference to pass to validations.
	c.imageRef = plugin.ImageReference{
		ImageURI:        c.Image,
		ImageFSPath:     containerFSPath,
		ImageInfo:       img,
		ImageRegistry:   reference.Context().RegistryStr(),
		ImageRepository: reference.Context().RepositoryStr(),
		ImageTagOrSha:   reference.Identifier(),
	}

	if err := writeCertImage(ctx, c.imageRef); err != nil {
		return fmt.Errorf("could not write cert image: %v", err)
	}

	if !c.IsScratch {
		if err := writeRPMManifest(ctx, containerFSPath); err != nil {
			return fmt.Errorf("could not write rpm manifest: %v", err)
		}
	}

	// if c.IsBundle {
	// 	// Record test cluster version
	// 	version, err := openshift.GetOpenshiftClusterVersion(ctx, c.Kubeconfig)
	// 	if err != nil {
	// 		logger.Error(err, "could not determine test cluster version")
	// 	}
	// 	c.results.TestedOn = version
	// } else {
	// 	logger.V(log.DBG).Info("Container checks do not require a cluster. skipping cluster version check.")
	// 	c.results.TestedOn = runtime.UnknownOpenshiftClusterVersion()
	// }

	// execute checks
	logger.V(log.DBG).Info("executing checks")
	for _, ch := range c.Checks {
		c.results.TestedImage = c.Image

		logger.V(log.DBG).Info("running check", "check", ch.Name())
		if ch.Metadata().Level == "optional" {
			logger.Info(fmt.Sprintf("Check %s is not currently being enforced.", ch.Name()))
		}

		// run the validation
		checkStartTime := time.Now()
		checkPassed, err := ch.Validate(ctx, c.imageRef)
		checkElapsedTime := time.Since(checkStartTime)

		if err != nil {
			logger.WithValues("result", "ERROR", "err", err.Error()).Info("check completed", "check", ch.Name())
			c.results.Errors = appendUnlessOptional(c.results.Errors, plugin.Result{Check: plugin.CheckInfo{
				Name:     ch.Name,
				Metadata: ch.Metadata,
				Help:     ch.Help,
			}, ElapsedTime: checkElapsedTime})
			continue
		}

		if !checkPassed {
			logger.WithValues("result", "FAILED").Info("check completed", "check", ch.Name())
			c.results.Failed = appendUnlessOptional(c.results.Failed, plugin.Result{Check: plugin.CheckInfo{
				Name:     ch.Name,
				Metadata: ch.Metadata,
				Help:     ch.Help,
			}, ElapsedTime: checkElapsedTime})
			continue
		}

		logger.WithValues("result", "PASSED").Info("check completed", "check", ch.Name())
		c.results.Passed = appendUnlessOptional(c.results.Passed, plugin.Result{Check: plugin.CheckInfo{
			Name:     ch.Name,
			Metadata: ch.Metadata,
			Help:     ch.Help,
		}, ElapsedTime: checkElapsedTime})

	}

	if len(c.results.Errors) > 0 || len(c.results.Failed) > 0 {
		c.results.PassedOverall = false
	} else {
		c.results.PassedOverall = true
	}

	// Inform the user of the tag-digest binding.
	// By this point, we should have already resolved the digest so
	// we don't handle this error, but fail safe and don't log a potentially
	// incorrect line message to the user.
	if resolvedDigest, err := c.imageRef.ImageInfo.Digest(); err == nil {
		msg, warn := tagDigestBindingInfo(c.imageRef.ImageTagOrSha, resolvedDigest.String())
		if warn {
			logger.Info(fmt.Sprintf("Warning: %s", msg))
		} else {
			logger.Info(msg)
		}
	}

	return nil
}

func appendUnlessOptional(results []plugin.Result, result plugin.Result) []plugin.Result {
	if result.Check.Metadata().Level == "optional" {
		return results
	}
	return append(results, result)
}

// tagDigestBindingInfo emits a log line describing tag and digest binding semantics.
// The providedIdentifer is the tag or digest of the image as the user gave it at the commandline.
// resolvedDigest
func tagDigestBindingInfo(providedIdentifier string, resolvedDigest string) (msg string, warn bool) {
	if strings.HasPrefix(providedIdentifier, "sha256:") {
		return "You've provided an image by digest. " +
				"When submitting this image to Red Hat for certification, " +
				"no tag will be associated with this image. " +
				"If you would like to associate a tag with this image, " +
				"please rerun this tool replacing your image reference with a tag.",
			true
	}

	return fmt.Sprintf(
		`This image's tag %s will be paired with digest %s `+
			`once this image has been published in accordance `+
			`with Red Hat Certification policy. `+
			`You may then add or remove any supplemental tags `+
			`through your Red Hat Connect portal as you see fit.`,
		providedIdentifier, resolvedDigest,
	), false
}

// Results will return the results of check execution.
func (c *CraneEngine) Results(ctx context.Context) plugin.Results {
	return c.results
}

// Untar takes a destination path and a reader; a tar reader loops over the tarfile
// creating the file structure at 'dst' along the way, and writing any files
func untar(ctx context.Context, dst string, r io.Reader) error {
	logger := logr.FromContextOrDiscard(ctx)
	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()

		switch {
		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {
		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0o755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()

			// if it's a link create it
		case tar.TypeSymlink:
			err := os.Symlink(header.Linkname, filepath.Join(dst, header.Name))
			if err != nil {
				logger.V(log.DBG).Info(fmt.Sprintf("Error creating link: %s. Ignoring.", header.Name))
				continue
			}
		}
	}
}

// writeCertImage takes imageRef and writes it to disk as JSON representing a pyxis.CertImage
// struct. The file is written at path certification.DefaultCertImageFilename.
//
//nolint:unparam // ctx is unused. Keep for future use.
func writeCertImage(ctx context.Context, imageRef plugin.ImageReference) error {
	logger := logr.FromContextOrDiscard(ctx)

	config, err := imageRef.ImageInfo.ConfigFile()
	if err != nil {
		return fmt.Errorf("failed to get image config file: %w", err)
	}

	manifest, err := imageRef.ImageInfo.Manifest()
	if err != nil {
		return fmt.Errorf("failed to get image manifest: %w", err)
	}

	digest, err := imageRef.ImageInfo.Digest()
	if err != nil {
		return fmt.Errorf("failed to get image digest: %w", err)
	}

	rawConfig, err := imageRef.ImageInfo.RawConfigFile()
	if err != nil {
		return fmt.Errorf("failed to image raw config file: %w", err)
	}

	size, err := imageRef.ImageInfo.Size()
	if err != nil {
		return fmt.Errorf("failed to get image size: %w", err)
	}

	labels := convertLabels(config.Config.Labels)
	layerSizes := make([]pyxis.Layer, 0, len(config.RootFS.DiffIDs))
	for _, diffid := range config.RootFS.DiffIDs {
		layer, err := imageRef.ImageInfo.LayerByDiffID(diffid)
		if err != nil {
			return fmt.Errorf("could not get layer by diff id: %w", err)
		}

		uncompressed, err := layer.Uncompressed()
		if err != nil {
			return fmt.Errorf("could not get uncompressed layer: %w", err)
		}
		written, err := io.Copy(io.Discard, uncompressed)
		if err != nil {
			return fmt.Errorf("could not copy from layer: %w", err)
		}

		pyxisLayer := pyxis.Layer{
			LayerID: diffid.String(),
			Size:    written,
		}
		layerSizes = append(layerSizes, pyxisLayer)
	}

	manifestLayers := make([]string, 0, len(manifest.Layers))
	for _, layer := range manifest.Layers {
		manifestLayers = append(manifestLayers, layer.Digest.String())
	}

	sumLayersSizeBytes := sumLayerSizeBytes(layerSizes)

	addedDate := time.Now().UTC().Format(time.RFC3339)

	tags := make([]pyxis.Tag, 0, 1)
	tags = append(tags, pyxis.Tag{
		AddedDate: addedDate,
		Name:      imageRef.ImageTagOrSha,
	})

	repositories := make([]pyxis.Repository, 0, 1)
	repositories = append(repositories, pyxis.Repository{
		PushDate:   addedDate,
		Registry:   imageRef.ImageRegistry,
		Repository: imageRef.ImageRepository,
		Tags:       tags,
	})

	certImage := pyxis.CertImage{
		DockerImageDigest: digest.String(),
		DockerImageID:     manifest.Config.Digest.String(),
		ImageID:           digest.String(),
		Architecture:      config.Architecture,
		ParsedData: &pyxis.ParsedData{
			Architecture:           config.Architecture,
			Command:                strings.Join(config.Config.Cmd, " "),
			Created:                config.Created.String(),
			DockerVersion:          config.DockerVersion,
			ImageID:                digest.String(),
			Labels:                 labels,
			Layers:                 manifestLayers,
			OS:                     config.OS,
			Size:                   size,
			UncompressedLayerSizes: layerSizes,
		},
		RawConfig:         string(rawConfig),
		Repositories:      repositories,
		SumLayerSizeBytes: sumLayersSizeBytes,
		// This is an assumption that the DiffIDs are in order from base up.
		// Need more evidence that this is always the case.
		UncompressedTopLayerID: config.RootFS.DiffIDs[0].String(),
	}

	// calling MarshalIndent so the json file written to disk is human-readable when opened
	certImageJSON, err := json.MarshalIndent(certImage, "", "    ")
	if err != nil {
		return fmt.Errorf("could not marshal cert image: %w", err)
	}

	artifactWriter := artifacts.WriterFromContext(ctx)
	if artifactWriter != nil {
		fileName, err := artifactWriter.WriteFile(defaults.DefaultCertImageFilename, bytes.NewReader(certImageJSON))
		if err != nil {
			return fmt.Errorf("failed to save file to artifacts directory: %w", err)
		}

		logger.V(log.TRC).Info("image config written to disk", "filename", fileName)
	}

	return nil
}

func getBgName(srcrpm string) string {
	parts := strings.Split(srcrpm, "-")
	return strings.Join(parts[0:len(parts)-2], "-")
}

func writeRPMManifest(ctx context.Context, containerFSPath string) error {
	logger := logr.FromContextOrDiscard(ctx)
	pkgList, err := rpm.GetPackageList(ctx, containerFSPath)
	if err != nil {
		logger.Error(err, "could not get rpm list, continuing without it")
	}

	// covert rpm struct to pxyis struct
	rpms := make([]pyxis.RPM, 0, len(pkgList))
	for _, packageInfo := range pkgList {
		var bgName, endChop, srpmNevra, pgpKeyID string

		// accounting for the fact that not all packages have a source rpm
		if len(packageInfo.SourceRpm) > 0 {
			bgName = getBgName(packageInfo.SourceRpm)
			endChop = strings.TrimPrefix(strings.TrimSuffix(regexp.MustCompile("(-[0-9].*)").FindString(packageInfo.SourceRpm), ".rpm"), "-")

			srpmNevra = fmt.Sprintf("%s-%d:%s", bgName, packageInfo.Epoch, endChop)
		}

		if len(packageInfo.PGP) > 0 {
			matches := regexp.MustCompile(".*, Key ID (.*)").FindStringSubmatch(packageInfo.PGP)
			if matches != nil {
				pgpKeyID = matches[1]
			} else {
				logger.V(log.DBG).Info("string did not match the format required", "pgp", packageInfo.PGP)
				pgpKeyID = ""
			}
		}

		pyxisRPM := pyxis.RPM{
			Architecture: packageInfo.Arch,
			Gpg:          pgpKeyID,
			Name:         packageInfo.Name,
			Nvra:         fmt.Sprintf("%s-%s-%s.%s", packageInfo.Name, packageInfo.Version, packageInfo.Release, packageInfo.Arch),
			Release:      packageInfo.Release,
			SrpmName:     bgName,
			SrpmNevra:    srpmNevra,
			Summary:      packageInfo.Summary,
			Version:      packageInfo.Version,
		}

		rpms = append(rpms, pyxisRPM)
	}

	rpmManifest := pyxis.RPMManifest{
		RPMS: rpms,
	}

	// calling MarshalIndent so the json file written to disk is human-readable when opened
	rpmManifestJSON, err := json.MarshalIndent(rpmManifest, "", "    ")
	if err != nil {
		return fmt.Errorf("could not marshal rpm manifest: %w", err)
	}

	if artifactWriter := artifacts.WriterFromContext(ctx); artifactWriter != nil {
		fileName, err := artifactWriter.WriteFile(defaults.DefaultRPMManifestFilename, bytes.NewReader(rpmManifestJSON))
		if err != nil {
			return fmt.Errorf("failed to save file to artifacts directory: %w", err)
		}

		logger.V(log.TRC).Info("rpm manifest written to disk", "filename", fileName)
	}

	return nil
}

func sumLayerSizeBytes(layers []pyxis.Layer) int64 {
	var sum int64
	for _, layer := range layers {
		sum += layer.Size
	}

	return sum
}

func convertLabels(imageLabels map[string]string) []pyxis.Label {
	pyxisLabels := make([]pyxis.Label, 0, len(imageLabels))
	for key, value := range imageLabels {
		label := pyxis.Label{
			Name:  key,
			Value: value,
		}

		pyxisLabels = append(pyxisLabels, label)
	}

	return pyxisLabels
}

// retryOnceAfter is a crane option that retries once after t duration.
func retryOnceAfter(t time.Duration) crane.Option {
	return func(o *crane.Options) {
		o.Remote = append(o.Remote, remote.WithRetryBackoff(remote.Backoff{
			Duration: t,
			Factor:   1.0,
			Jitter:   0.1,
			Steps:    2,
		}))
	}
}

// // ContainerCheckConfig contains configuration relevant to an individual check's execution.
// type ContainerCheckConfig struct {
// 	DockerConfig, PyxisAPIToken, CertificationProjectID string
// }

// // InitializeContainerChecks returns the appropriate checks for policy p given cfg.
// func InitializeContainerChecks(ctx context.Context, p policy.Policy, cfg ContainerCheckConfig) ([]types.Check, error) {
// 	switch p {
// 	case policy.PolicyContainer:
// 		return []types.Check{
// 			&policy.HasLicenseCheck{},
// 			// 	policy.NewHasUniqueTagCheck(cfg.DockerConfig),
// 			// 	&policy.MaxLayersCheck{},
// 			// 	&policy.HasNoProhibitedPackagesCheck{},
// 			// 	&policy.HasRequiredLabelsCheck{},
// 			// 	&policy.RunAsNonRootCheck{},
// 			// 	&policy.HasModifiedFilesCheck{},
// 			// 	policy.NewBasedOnUbiCheck(pyxis.NewPyxisClient(
// 			// 		check.DefaultPyxisHost,
// 			// 		cfg.PyxisAPIToken,
// 			// 		cfg.CertificationProjectID,
// 			// 		&http.Client{Timeout: 60 * time.Second})),
// 		}, nil
// 	case policy.PolicyRoot:
// 		return []types.Check{
// 			&policy.HasLicenseCheck{},
// 			// policy.NewHasUniqueTagCheck(cfg.DockerConfig),
// 			// &policy.MaxLayersCheck{},
// 			// &policy.HasNoProhibitedPackagesCheck{},
// 			// &policy.HasRequiredLabelsCheck{},
// 			// &policy.HasModifiedFilesCheck{},
// 			// policy.NewBasedOnUbiCheck(pyxis.NewPyxisClient(
// 			// 	check.DefaultPyxisHost,
// 			// 	cfg.PyxisAPIToken,
// 			// 	cfg.CertificationProjectID,
// 			// 	&http.Client{Timeout: 60 * time.Second})),
// 		}, nil
// 	case policy.PolicyScratch:
// 		return []types.Check{
// 			&policy.HasLicenseCheck{},
// 			// policy.NewHasUniqueTagCheck(cfg.DockerConfig),
// 			// &policy.MaxLayersCheck{},
// 			// &policy.HasRequiredLabelsCheck{},
// 			// &policy.RunAsNonRootCheck{},
// 		}, nil
// 	}

// 	return nil, fmt.Errorf("provided container policy %s is unknown", p)
// }

// // makeCheckList returns a list of check names.
// func makeCheckList(checks []types.Check) []string {
// 	checkNames := make([]string, len(checks))

// 	for i, check := range checks {
// 		checkNames[i] = check.Name()
// 	}

// 	return checkNames
// }

// // ContainerPolicy returns the names of checks in the container policy.
// func ContainerPolicy(ctx context.Context) []string {
// 	return checkNamesFor(ctx, policy.PolicyContainer)
// }

// // ScratchContainerPolicy returns the names of checks in the
// // container policy with scratch exception.
// func ScratchContainerPolicy(ctx context.Context) []string {
// 	return checkNamesFor(ctx, policy.PolicyScratch)
// }

// // RootExceptionContainerPolicy returns the names of checks in the
// // container policy with root exception.
// func RootExceptionContainerPolicy(ctx context.Context) []string {
// 	return checkNamesFor(ctx, policy.PolicyRoot)
// }
