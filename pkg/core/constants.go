package core

const (
	EventsDirName string = "events"
	TagsDirName   string = "tags"

	GitAuthorName string = "git-calendar"
	GitRemoteName string = "origin"
	GitBranchName string = "main"

	KeyFileSuffix      = ".key"
	ReadonlyFileSuffix = ".readonly"
	ICalURLFileSuffix  = ".url"
	ICalFileSuffix     = ".ics"

	// IndexFileName     string = "index.json"
	// RichIndexFileName string = "index-rich.json"
)

type UpdateStrategy int

const (
	Current UpdateStrategy = iota
	Following
	All
)

func (opt UpdateStrategy) IsValid() bool {
	return opt >= Current && opt <= All
}
