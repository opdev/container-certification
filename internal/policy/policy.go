package policy

type Policy = string

const (
	PolicyContainer Policy = "container"
	PolicyScratch   Policy = "scratch"
	PolicyRoot      Policy = "root"
)
