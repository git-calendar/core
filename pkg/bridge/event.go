package gocore

// A struct for Kotlin/Swift to use as the event structure.
type Event struct {
	Id       string `json:"id"` // (not using uuid.UUID for cross lang. compatibility)
	Title    string `json:"title"`
	Location string `json:"location"`
	From     string `json:"from"` // RFC3339 format e.g. 2009-11-10T23:00:00Z (the default format for json.Marshal() when it comes to time.Time)
	To       string `json:"to"`   // RFC3339 format e.g. 2009-11-10T23:00:00Z (the default format for json.Marshal() when it comes to time.Time)
}
