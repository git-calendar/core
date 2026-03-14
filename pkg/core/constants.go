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

type EventTree = *interval.SearchTree[[]uuid.UUID, time.Time] // for each interval we can have multiple events; <time.Time, time.Time> -> []uuid.UUID

// ------- Repeating frequency -------

// Repeating frequency.
type Freq int

const (
	Invalid Freq = iota // ints default value 0 is invalid
	Day                 // Repeat daily.
	Week                // Repeat weekly.
	Month               // Repeat monthly.
	Year                // Repeat yearly.
	_max                // boundary for validation
)

func (t Freq) IsValid() bool {
	return t > Invalid && t <= _max
}

// ------- Repeating update option -------

type UpdateOption int

const (
	Current UpdateOption = iota
	Following
	All
)

func (opt UpdateOption) IsValid() bool {
	return opt >= Current && opt <= All
}

func ParseUpdateOption(strategy string) UpdateOption {
	switch strings.ToLower(strategy) {
	case "current":
		return Current
	case "following":
		return Following
	case "all":
		return All
	default:
		return Current
	}
}
