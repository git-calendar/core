package api

// A DTO (we love Java) for Kotlin/Swift to use as the event structure.
//
// This event isn't used in Go itself, but serves as a "shape definition" for `gomobile` to bind it into Kotlin/Swift.
type Event struct {
	Id          string
	Title       string
	Location    string
	Description string
	From        string // RFC3339 format e.g., 2009-11-10T23:00:00Z (the default format of json.Marshal() for time.Time)
	To          string // RFC3339 format e.g., 2009-11-10T23:00:00Z (the default format of json.Marshal() for time.Time)
	Calendar    string
	TagId       string
	ParentId    string
	Repeat      *Repetition
}

type Repetition struct {
	Frequency  int
	Interval   int
	Until      string
	Count      int
	Exceptions []string
}

// A DTO for Kotlin/Swift to use as the calendar structure.
//
// This calendar isn't used in Go itself, but serves as a "shape definition" for `gomobile` to bind it into Kotlin/Swift.
type Calendar struct {
	Name      string
	Tags      []Tag
	RemoteUrl string
	Encrypted bool
	Readonly  bool
}

type Tag struct {
	Id    string
	Name  string
	Color string
}
