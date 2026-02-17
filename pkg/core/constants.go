package core

const (
	IndexFileName     string = "index.json"
	RichIndexFileName string = "index-rich.json"

	EventsDirName string = "events"
	// GroupsDirName string = "groups"
)

type TimeUnit int

const (
	Day TimeUnit = iota
	Week
	Month
	Year
)

func (t TimeUnit) IsValid() bool {
	return t >= Day && t <= Year
}

type UpdateOption int

const (
	Current UpdateOption = iota
	Following
	All
)

func (opt UpdateOption) IsValid() bool {
	return opt >= Current && opt <= All
}
