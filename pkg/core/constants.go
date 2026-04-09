package core

const (
	IndexFileName         string = "index.json"
	RichIndexFileName     string = "index-rich.json"
	EncryptionKeyFileName string = "encryption.key"

	EventsDirName string = "events"

	GitAuthorName string = "git-calendar"
)

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
