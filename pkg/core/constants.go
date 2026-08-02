package core

const (
	// EventsDirName is the repository directory containing event files.
	EventsDirName string = "events"
	// TagsDirName is the repository directory containing tag files.
	TagsDirName string = "tags"

	// GitAuthorName is the author name used for generated commits.
	GitAuthorName string = "git-calendar"
	// GitRemoteName is the managed Git remote name.
	GitRemoteName string = "origin"
	// GitBranchName is the managed Git branch name.
	GitBranchName string = "main"

	// KeyFileSuffix identifies calendar encryption-key files.
	KeyFileSuffix string = ".key"
	// ReadonlyFileSuffix identifies calendar read-only marker files.
	ReadonlyFileSuffix string = ".readonly"
	// ICalURLFileSuffix identifies iCalendar source URL files.
	ICalURLFileSuffix string = ".url"
	// ICalFileSuffix identifies cached iCalendar files.
	ICalFileSuffix string = ".ics"

	// IndexFileName     string = "index.json"
	// RichIndexFileName string = "index-rich.json"
)

// UpdateStrategy selects which occurrences of a recurring event to change.
type UpdateStrategy int

const (
	// Current applies a change only to the selected occurrence.
	Current UpdateStrategy = iota
	// Following applies a change to the selected and later occurrences.
	Following
	// All applies a change to the entire recurring series.
	All
)

// IsValid reports whether the update strategy is supported.
func (opt UpdateStrategy) IsValid() bool {
	return opt >= Current && opt <= All
}
