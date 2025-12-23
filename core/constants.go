package core

// calendar repo structure:
//
//	git-calendar-data
//	- index.jsonl
//	- index-rich.jsonl
//	- events/
//	  - UUID.json
//	- groups/
//	  - UUID.json

const (
	IndexFileName     string = "index.json"
	RichIndexFileName string = "index-rich.json"

	RootDirName   string = "git-calendar-data"
	EventsDirName string = "events"
	GroupsDirName string = "groups"
)
