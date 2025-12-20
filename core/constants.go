package core

// calendar repo structure:
//
//	root
//	- index.jsonl
//	- rich-index.jsonl
//	- events/
//	  - UUID.json
//	- groups/
//	  - UUID.json

const (
	IndexFileName     string = "index.json"
	RichIndexFileName string = "rich-index.json"
	EventsDirName     string = "events"
	GroupsDirName     string = "groups"
)
