package core

const (
	IndexFileName     string = "index.json"
	RichIndexFileName string = "index-rich.json"

	EventsDirName string = "events"
	// GroupsDirName string = "groups"
)

type TimeUnit int

const (
	None TimeUnit = iota
	Day
	Week
	TwoWeeks
	Month
	Year
)
