package core

const (
	// IndexFileName     string = "index.json"
	// RichIndexFileName string = "index-rich.json"

	EventsDirName string = "events"
	TagsDirName          = "tags"

	GitAuthorName string = "git-calendar"
	GitRemoteName string = "origin"
	GitBranchName string = "main"
)

// ------- Repeating update strategy -------

type UpdateStrategy int

const (
	Current UpdateStrategy = iota
	Following
	All
)

func (opt UpdateStrategy) IsValid() bool {
	return opt >= Current && opt <= All
}
