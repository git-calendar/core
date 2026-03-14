package core

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rdleal/intervalst/interval"
)

const (
	IndexFileName     string = "index.json"
	RichIndexFileName string = "index-rich.json"

	EventsDirName string = "events"
)

type EventTree = *interval.SearchTree[[]uuid.UUID, time.Time]

type Freq int

const (
	Day Freq = iota
	Week
	Month
	Year
)

func (t Freq) IsValid() bool {
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
