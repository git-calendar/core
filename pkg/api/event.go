package api

// A DTO (we love Java) for Kotlin/Swift to use as the event structure.
//
// This event isn't used in Go itself, but serves as a "shape definition" for `gomobile` to bind it into Kotlin/Swift.
type Event struct {
	Id               string   `json:"id"`
	Title            string   `json:"title"`
	Location         string   `json:"location"`
	From             string   `json:"from"` // RFC3339 format e.g. 2009-11-10T23:00:00Z (the default format for json.Marshal() when it comes to time.Time)
	To               string   `json:"to"`   // RFC3339 format e.g. 2009-11-10T23:00:00Z (the default format for json.Marshal() when it comes to time.Time)
	Duration         string   `json:"duration"`
	Notes            string   `json:"notes"`
	Repetition       int      `json:"repetition"`
	RepeatExceptions []string `json:"repeat_exceptions"`
	ParentId         string   `json:"parent_id"`
}
