package core

import "strings"

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

func ParseUpdateOption(strategy string) UpdateOption {
	switch strings.ToLower(strategy) {
	case "following":
		return Following
	case "all":
		return All
	case "current":
		return Current
	default:
		return Current
	}
}

func (opt UpdateOption) IsValid() bool {
	return opt >= Current && opt <= All
}
