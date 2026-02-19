package api

// A DTO (we love Java) for Kotlin/Swift to use as the event structure.
//
// This event isn't used in Go itself, but serves as a "shape definition" for `gomobile` to bind it into Kotlin/Swift.
type Event struct {
	Id          string      `json:"id"`
	Title       string      `json:"title"`
	Location    string      `json:"location"`
	Description string      `json:"description"`
	From        string      `json:"from"` // RFC3339 format e.g. 2009-11-10T23:00:00Z (the default format for json.Marshal() when it comes to time.Time)
	To          string      `json:"to"`   // RFC3339 format e.g. 2009-11-10T23:00:00Z (the default format for json.Marshal() when it comes to time.Time)
	MasterId    string      `json:"masterId"`
	Repeat      *Repetition `json:"repeat"`
}

type Repetition struct {
	Frequency  int        `json:"frequency"`
	Interval   int        `json:"interval"`
	Until      string     `json:"until"`
	Count      int        `json:"count"`
	Exceptions *Exception `json:"exceptions"`
}

type Exception struct {
	Id   string `json:"id"`
	Time string `json:"time"`
}
