// A JSON API wrapper around the core.Core for multiplatform support.
// It's not possible to expose any "complex" data types (structs*, arrays, channels, maps, etc.),
// because they do not have bindings to other languages.
// Let's use JSON everywhere as a REST API would...
//
// (*) You can return a *Event (pointer to struct), but you cannot receive it as argument.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/git-calendar/core/pkg/core"
	"github.com/google/uuid"
	"github.com/teambition/rrule-go"
)

const (
	emptyJson    = "{}"
	emptyJsonArr = "[]"
)

// The exposed/exported JSON-only API interface.
type Api struct {
	inner *core.Core
}

// eventJSON is the API representation of an event. Recurrence is transported
// as an RFC 5545 string because rrule.Set is not JSON-serializable.
type eventJSON struct {
	core.Event
	Repeat *string `json:"repeat"`
}

// A "constructor" for the JSON API.
func NewApi() *Api {
	return &Api{
		inner: core.NewCore(),
	}
}

// -------------------------- Boring methods that do not need any json parsing etc. -------------------------

func (a *Api) CreateCalendar(name, password string) error {
	return a.inner.CreateCalendar(name, password)
}
func (a *Api) RemoveCalendar(name string) error { return a.inner.RemoveCalendar(name) }
func (a *Api) RenameCalendar(oldName, newName string) error {
	return a.inner.RenameCalendar(oldName, newName)
}
func (a *Api) LoadCalendars() error                      { return a.inner.LoadCalendars() }
func (a *Api) SetCorsProxy(proxyUrl string) error        { return a.inner.SetCorsProxy(proxyUrl) }
func (a *Api) SyncAll() error                            { return a.inner.SyncAll() }
func (a *Api) ExportZip(calendar string) ([]byte, error) { return a.inner.ExportZip(calendar) }
func (a *Api) ImportICalFile(calendar, data string) error {
	return a.inner.ImportICalFile(calendar, strings.NewReader(data))
}

// ------------------------------  Wrapper methods encoding and decoding JSONs ------------------------------

func (a *Api) ImportICalURL(name, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("iCalendar URL is invalid: %w", err)
	}
	return a.inner.ImportICalURL(name, parsed)
}

func (a *Api) UpdateRemote(calendar string, remoteUrl string, readonly bool) error {
	parsed, err := url.Parse(remoteUrl)
	if err != nil {
		return fmt.Errorf("remoteUrl is invalid: %w", err)
	}
	return a.inner.UpdateRemote(calendar, parsed, readonly)
}

func (a *Api) CloneCalendar(repoUrl, password string, readonly bool) error {
	parsedUrl, err := url.Parse(repoUrl)
	if err != nil {
		return fmt.Errorf("repoUrl is invalid: %w", err)
	}
	return a.inner.CloneCalendar(parsedUrl, password, readonly)
}

func (a *Api) ListCalendars() (string, error) {
	arr, err := a.inner.ListCalendars()
	if err != nil {
		return emptyJsonArr, err
	}
	data, err := json.Marshal(arr)
	if err != nil {
		return emptyJsonArr, fmt.Errorf("failed to marshal names to json: %w", err)
	}
	return string(data), nil
}

func (a *Api) CreateEvent(eventJson string) (string, error) {
	return returnJsonEventAndError(eventJson, a.inner.CreateEvent)
}

func (a *Api) UpdateEvent(eventJson string) (string, error) {
	return returnJsonEventAndError(eventJson, a.inner.UpdateEvent)
}

func (a *Api) UpdateRepeatingEvent(oldEventJson, newEventJson string, strategy int) (string, error) {
	oldEvent, err := unmarshalEvent(oldEventJson)
	if err != nil {
		return emptyJson, err
	}
	newEvent, err := unmarshalEvent(newEventJson)
	if err != nil {
		return emptyJson, err
	}

	updated, err := a.inner.UpdateRepeatingEvent(oldEvent, newEvent, core.UpdateStrategy(strategy))
	if err != nil {
		return emptyJson, err
	}
	return marshalEvent(updated)
}

func (a *Api) RemoveEvent(eventJson string) error {
	event, err := unmarshalEvent(eventJson)
	if err != nil {
		return err
	}
	return a.inner.RemoveEvent(event)
}

func (a *Api) RemoveRepeatingEvent(eventJson string, strategy int) error {
	event, err := unmarshalEvent(eventJson)
	if err != nil {
		return err
	}
	return a.inner.RemoveRepeatingEvent(event, core.UpdateStrategy(strategy))
}

func (a *Api) GetEvent(id string) (string, error) {
	parsedId, err := uuid.Parse(id)
	if err != nil {
		return emptyJson, fmt.Errorf("invalid event id: %w", err)
	}
	// pass the id to inner api
	event, err := a.inner.GetEvent(parsedId)
	if err != nil {
		return emptyJson, err
	}

	return marshalEvent(event)
}

func (a *Api) GetEvents(from, to string, filterJson string) (string, error) {
	// parse both time strings
	f, err1 := time.Parse(time.RFC3339, from)
	t, err2 := time.Parse(time.RFC3339, to)
	if err := errors.Join(err1, err2); err != nil {
		return emptyJsonArr, fmt.Errorf("invalid from/to parameter: %w", err)
	}

	var filter core.GetEventsFilter
	if filterJson != "" {
		if err := json.Unmarshal([]byte(filterJson), &filter); err != nil {
			return emptyJsonArr, err
		}
	}

	// pass the args to inner api
	events := a.inner.GetEvents(f, t, filter)

	return marshalEvents(events)
}

func (a *Api) CreateTag(calendar, tagJson string) (string, error) {
	var tag core.Tag
	err := json.Unmarshal([]byte(tagJson), &tag)
	if err != nil {
		fmt.Println("CalendarCore got: ", tagJson)
		return emptyJson, fmt.Errorf("failed to unmarshal tag data: %w", err)
	}

	newTag, err := a.inner.CreateTag(calendar, tag)
	if err != nil {
		fmt.Println("CalendarCore got: ", tagJson)
		return emptyJson, err
	}

	jsonBytes, err := json.Marshal(newTag)
	if err != nil {
		return emptyJson, err
	}

	return string(jsonBytes), nil
}

func (a *Api) UpdateTag(calendar, tagJson string) (string, error) {
	var tag core.Tag
	err := json.Unmarshal([]byte(tagJson), &tag)
	if err != nil {
		fmt.Println("CalendarCore got: ", tagJson)
		return emptyJson, fmt.Errorf("failed to unmarshal tag data: %w", err)
	}

	newTag, err := a.inner.UpdateTag(calendar, tag)
	if err != nil {
		fmt.Println("CalendarCore got: ", tagJson)
		return emptyJson, err
	}

	jsonBytes, err := json.Marshal(newTag)
	if err != nil {
		return emptyJson, err
	}

	return string(jsonBytes), nil
}

func (a *Api) RemoveTag(calendar, id string) error {
	parsedId, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid tag id: %w", err)
	}

	return a.inner.RemoveTag(calendar, parsedId)
}

// ------------------------------------------------ Helpers -------------------------------------------------

func returnJsonEventAndError(eventJson string, coreFunc func(core.Event) (*core.Event, error)) (string, error) {
	event, err := unmarshalEvent(eventJson)
	if err != nil {
		return emptyJson, err
	}
	updated, err := coreFunc(event)
	if err != nil {
		return emptyJson, err
	}
	return marshalEvent(updated)
}

// unmarshalEvent decodes API JSON and rebuilds the internal recurrence set.
func unmarshalEvent(raw string) (core.Event, error) {
	var data eventJSON
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return core.Event{}, fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	var repeat *rrule.Set
	if data.Repeat != nil && *data.Repeat != "" {
		var err error
		repeat, err = rrule.StrToRRuleSet(*data.Repeat)
		if err != nil {
			return core.Event{}, fmt.Errorf("invalid recurrence: %w", err)
		}
	}
	data.Event.Repeat = repeat
	return data.Event, nil
}

// marshalEvent converts the recurrence set to its string form and encodes the event.
func marshalEvent(event *core.Event) (string, error) {
	data, err := json.Marshal(eventToJSON(*event))
	if err != nil {
		return emptyJson, fmt.Errorf("failed to marshal event data: %w", err)
	}
	return string(data), nil
}

func marshalEvents(events []core.Event) (string, error) {
	result := make([]eventJSON, len(events))
	for i := range events {
		result[i] = eventToJSON(events[i])
	}
	data, err := json.Marshal(result)
	if err != nil {
		return emptyJsonArr, fmt.Errorf("failed to marshal event data: %w", err)
	}
	return string(data), nil
}

func eventToJSON(event core.Event) eventJSON {
	var repeat *string
	if event.Repeat != nil {
		value := event.Repeat.String()
		repeat = &value
	}
	return eventJSON{Event: event, Repeat: repeat}
}
