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
	"time"

	"github.com/firu11/git-calendar-core/pkg/core"
	"github.com/google/uuid"
)

const (
	emptyJson    = "{}"
	emptyJsonArr = "[]"
)

// The exposed/exported JSON-only API interface.
type Api struct {
	inner *core.Core
}

// A "constructor" for the JSON API.
func NewApi() *Api {
	return &Api{
		inner: core.NewCore(),
	}
}

// -------------------------- Boring methods that do not need any json parsing etc. -------------------------

// func (a *Api) AddRemote(name, remoteUrl string) error { return a.inner.AddRemote(name, remoteUrl) }
func (a *Api) CreateCalendar(name string) error   { return a.inner.CreateCalendar(name) }
func (a *Api) RemoveCalendar(name string) error   { return a.inner.RemoveCalendar(name) }
func (a *Api) SetCorsProxy(proxyUrl string) error { return a.inner.SetCorsProxy(proxyUrl) }
func (a *Api) LoadCalendars() error               { return a.inner.LoadCalendars() }
func (a *Api) PullAll() error                     { return a.inner.PullAll() }
func (a *Api) PushAll() error                     { return a.inner.PushAll() }

// ------------------------------  Wrapper methods encoding and decoding JSONs ------------------------------

func (a *Api) CloneCalendar(repoUrl string) error {
	parsedUrl, err := url.Parse(repoUrl)
	if err != nil {
		return fmt.Errorf("repoUrl is invalid: %w", err)
	}
	return a.inner.CloneCalendar(*parsedUrl)
}

func (a *Api) ListCalendars() (string, error) {
	arr := a.inner.ListCalendars()
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
	var oldEvent core.Event
	var newEvent core.Event

	if err := json.Unmarshal([]byte(oldEventJson), &oldEvent); err != nil {
		fmt.Printf("CalendarCore got:\nNew: %s\nOld: %s\n", oldEventJson, newEventJson)
		return emptyJson, fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	if err := json.Unmarshal([]byte(newEventJson), &newEvent); err != nil {
		fmt.Printf("CalendarCore got:\nNew: %s\nOld: %s\n", oldEventJson, newEventJson)
		return emptyJson, fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	updatedEvent, err := a.inner.UpdateRepeatingEvent(oldEvent, newEvent, core.UpdateStrategy(strategy))
	if err != nil {
		fmt.Printf("CalendarCore got:\nNew: %s\nOld: %s\n", oldEventJson, newEventJson)
		return emptyJson, err
	}

	jsonBytes, err := json.Marshal(updatedEvent)
	if err != nil {
		return emptyJson, err
	}

	return string(jsonBytes), err
}

func (a *Api) RemoveEvent(eventJson string) error {
	var event core.Event
	err := json.Unmarshal([]byte(eventJson), &event)
	if err != nil {
		return fmt.Errorf("failed to unmarshal event data: %w", err)
	}
	return a.inner.RemoveEvent(event)
}

func (a *Api) RemoveRepeatingEvent(eventJson string, strategy int) error {
	var event core.Event
	err := json.Unmarshal([]byte(eventJson), &event)
	if err != nil {
		return fmt.Errorf("failed to unmarshal event data: %w", err)
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

	// marshal to json
	jsonBytes, err := json.Marshal(event)
	if err != nil {
		return emptyJson, fmt.Errorf("failed to marshal event to json: %w", err)
	}

	return string(jsonBytes), nil
}

func (a *Api) GetEvents(from, to string) (string, error) {
	// parse both time strings
	f, err1 := time.Parse(time.RFC3339, from)
	t, err2 := time.Parse(time.RFC3339, to)
	if err := errors.Join(err1, err2); err != nil {
		return emptyJsonArr, fmt.Errorf("invalid from/to parameter: %w", err)
	}

	// pass the args to inner api
	events := a.inner.GetEvents(f, t)

	// marshal to json
	jsonBytes, err := json.Marshal(events)
	if err != nil {
		return emptyJsonArr, fmt.Errorf("failed to marshal events to json: %w", err)
	}

	return string(jsonBytes), nil
}

// ------------------------------------------------ Helpers -------------------------------------------------

// A helper which:
//  1. Parses and validates input event
//  2. Calls the coreFunc
//  3. Marshals event that came back to JSON
//  4. Returns json
func returnJsonEventAndError(eventJson string, coreFunc func(core.Event) (*core.Event, error)) (string, error) {
	var event core.Event
	err := json.Unmarshal([]byte(eventJson), &event)
	if err != nil {
		fmt.Println("CalendarCore got: ", eventJson)
		return emptyJson, fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	newEvent, err := coreFunc(event)
	if err != nil {
		fmt.Println("CalendarCore got: ", eventJson)
		return emptyJson, err
	}

	jsonBytes, err := json.Marshal(newEvent)
	if err != nil {
		return emptyJson, err
	}

	return string(jsonBytes), err
}
