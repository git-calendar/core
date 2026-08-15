// Package api exposes a binding-friendly wrapper around core.Core.
//
// Domain values use JSON because generated mobile bindings cannot consistently
// represent arbitrary Go structs, maps, channels, or complex parameters. Some
// pointer results can bind even when equivalent parameters cannot, so domain
// input and output stay symmetric through JSON. Simple values and byte slices
// use native mobile and WebAssembly binding types.
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
	emptyJSON      = "{}"
	emptyJSONArray = "[]"
)

// Api wraps core.Core in a binding-friendly API.
//
// Api values must be created with NewApi.
type Api struct {
	inner *core.Core
}

// eventJSON is the API representation of an event. Recurrence is transported
// as an RFC 5545 string because rrule.Set is not JSON-serializable.
type eventJSON struct {
	core.Event
	Repeat *string `json:"repeat"`
}

// NewApi creates a binding-friendly API backed by a new core.Core.
func NewApi() *Api { return &Api{inner: core.NewCore()} }

// ----------------------------------------- Direct wrapper methods -----------------------------------------

// CreateCalendar creates a calendar with the given name and password.
func (a *Api) CreateCalendar(name, password string) error {
	return a.inner.CreateCalendar(name, password)
}

// RemoveCalendar removes the named calendar.
func (a *Api) RemoveCalendar(name string) error { return a.inner.RemoveCalendar(name) }

// RenameCalendar renames a calendar.
func (a *Api) RenameCalendar(oldName, newName string) error {
	return a.inner.RenameCalendar(oldName, newName)
}

// LoadCalendars reloads calendars from storage.
func (a *Api) LoadCalendars() error { return a.inner.LoadCalendars() }

// SetCorsProxy configures the CORS proxy used for remote requests.
func (a *Api) SetCorsProxy(proxyURL string) error { return a.inner.SetCorsProxy(proxyURL) }

// SyncAll synchronizes all configured calendars.
func (a *Api) SyncAll() error { return a.inner.SyncAll() }

// ExportZip exports one Git-backed calendar, or all persisted data when calendar is empty, as a ZIP archive.
// URL-backed calendars cannot be exported individually.
func (a *Api) ExportZip(calendar string) ([]byte, error) { return a.inner.ExportZip(calendar) }

// RestoreZip replaces all persisted data with a full ZIP backup.
func (a *Api) RestoreZip(data []byte) error { return a.inner.RestoreZip(data) }

// ------------------------------------------ JSON wrapper methods ------------------------------------------

// ImportICalURL imports an iCalendar feed URL under the given name.
func (a *Api) ImportICalURL(name, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("iCalendar URL is invalid: %w", err)
	}
	return a.inner.ImportICalURL(name, parsed)
}

// UpdateICalURL changes the source URL of an imported iCalendar.
func (a *Api) UpdateICalURL(name, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("iCalendar URL is invalid: %w", err)
	}
	return a.inner.UpdateICalURL(name, parsed)
}

// ImportICalFile imports iCalendar data into a calendar using the given tag ID.
// An empty tag ID imports events without a tag.
func (a *Api) ImportICalFile(calendar, tagID, data string) error {
	var parsedID *uuid.UUID
	if tagID != "" {
		id, err := uuid.Parse(tagID)
		if err != nil {
			return fmt.Errorf("invalid tag ID: %w", err)
		}
		parsedID = &id
	}
	return a.inner.ImportICalFile(calendar, parsedID, strings.NewReader(data))
}

// UpdateRemote configures the Git remote for a calendar.
// An empty remote URL removes the configured remote.
func (a *Api) UpdateRemote(calendar, remoteURL string, readonly bool) error {
	if remoteURL == "" {
		return a.inner.UpdateRemote(calendar, nil, readonly)
	}
	parsed, err := url.Parse(remoteURL)
	if err != nil {
		return fmt.Errorf("remote URL is invalid: %w", err)
	}
	return a.inner.UpdateRemote(calendar, parsed, readonly)
}

// CloneCalendar clones a remote calendar repository.
func (a *Api) CloneCalendar(repoURL, password string, readonly bool) error {
	parsedURL, err := url.Parse(repoURL)
	if err != nil {
		return fmt.Errorf("repository URL is invalid: %w", err)
	}
	return a.inner.CloneCalendar(parsedURL, password, readonly)
}

// ListCalendars returns the calendars as a JSON array.
func (a *Api) ListCalendars() (string, error) {
	calendars, err := a.inner.ListCalendars()
	if err != nil {
		return emptyJSONArray, err
	}
	data, err := json.Marshal(calendars)
	if err != nil {
		return emptyJSONArray, fmt.Errorf("failed to marshal calendars to JSON: %w", err)
	}
	return string(data), nil
}

// CreateEvent creates an event from its JSON representation and returns the created event as JSON.
func (a *Api) CreateEvent(eventJSON string) (string, error) {
	return returnJSONEventAndError(eventJSON, a.inner.CreateEvent)
}

// UpdateEvent updates an event from its JSON representation and returns the updated event as JSON.
func (a *Api) UpdateEvent(eventJSON string) (string, error) {
	return returnJSONEventAndError(eventJSON, a.inner.UpdateEvent)
}

// UpdateRepeatingEvent updates a repeating event using the requested strategy.
func (a *Api) UpdateRepeatingEvent(oldEventJSON, newEventJSON string, strategy int) (string, error) {
	oldEvent, err := unmarshalEvent(oldEventJSON)
	if err != nil {
		return emptyJSON, err
	}
	newEvent, err := unmarshalEvent(newEventJSON)
	if err != nil {
		return emptyJSON, err
	}

	updated, err := a.inner.UpdateRepeatingEvent(oldEvent, newEvent, core.UpdateStrategy(strategy))
	if err != nil {
		return emptyJSON, err
	}
	return marshalEvent(updated)
}

// RemoveEvent removes the event represented by the given JSON.
func (a *Api) RemoveEvent(eventJSON string) error {
	event, err := unmarshalEvent(eventJSON)
	if err != nil {
		return err
	}
	return a.inner.RemoveEvent(event)
}

// RemoveRepeatingEvent removes a repeating event using the requested strategy.
func (a *Api) RemoveRepeatingEvent(eventJSON string, strategy int) error {
	event, err := unmarshalEvent(eventJSON)
	if err != nil {
		return err
	}
	return a.inner.RemoveRepeatingEvent(event, core.UpdateStrategy(strategy))
}

// GetEvent returns an event as JSON by its ID.
func (a *Api) GetEvent(id string) (string, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return emptyJSON, fmt.Errorf("invalid event ID: %w", err)
	}

	// Pass the parsed ID to the Go-native core API.
	event, err := a.inner.GetEvent(parsedID)
	if err != nil {
		return emptyJSON, err
	}

	return marshalEvent(event)
}

// GetEvents returns events in an RFC 3339 interval as a JSON array.
// filterJSON may be empty or contain a core.GetEventsFilter JSON object.
func (a *Api) GetEvents(from, to, filterJSON string) (string, error) {
	// Parse both interval boundaries before calling the core API.
	f, err1 := time.Parse(time.RFC3339, from)
	t, err2 := time.Parse(time.RFC3339, to)
	if err := errors.Join(err1, err2); err != nil {
		return emptyJSONArray, fmt.Errorf("invalid from/to parameter: %w", err)
	}

	var filter core.GetEventsFilter
	if filterJSON != "" {
		if err := json.Unmarshal([]byte(filterJSON), &filter); err != nil {
			return emptyJSONArray, err
		}
	}

	// Pass decoded arguments to the Go-native core API.
	return marshalEvents(a.inner.GetEvents(f, t, filter))
}

// CreateTag creates a tag from its JSON representation and returns it as JSON.
func (a *Api) CreateTag(calendar, tagJSON string) (string, error) {
	var tag core.Tag
	if err := json.Unmarshal([]byte(tagJSON), &tag); err != nil {
		return emptyJSON, fmt.Errorf("failed to unmarshal tag data: %w", err)
	}

	newTag, err := a.inner.CreateTag(calendar, tag)
	if err != nil {
		return emptyJSON, err
	}

	data, err := json.Marshal(newTag)
	if err != nil {
		return emptyJSON, err
	}

	return string(data), nil
}

// UpdateTag updates a tag from its JSON representation and returns it as JSON.
func (a *Api) UpdateTag(calendar, tagJSON string) (string, error) {
	var tag core.Tag
	if err := json.Unmarshal([]byte(tagJSON), &tag); err != nil {
		return emptyJSON, fmt.Errorf("failed to unmarshal tag data: %w", err)
	}

	updatedTag, err := a.inner.UpdateTag(calendar, tag)
	if err != nil {
		return emptyJSON, err
	}

	data, err := json.Marshal(updatedTag)
	if err != nil {
		return emptyJSON, err
	}

	return string(data), nil
}

// RemoveTag removes a tag by its ID.
func (a *Api) RemoveTag(calendar, id string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid tag ID: %w", err)
	}

	return a.inner.RemoveTag(calendar, parsedID)
}

// ------------------------------------------------ Helpers -------------------------------------------------

func returnJSONEventAndError(eventJSON string, coreFunc func(core.Event) (*core.Event, error)) (string, error) {
	event, err := unmarshalEvent(eventJSON)
	if err != nil {
		return emptyJSON, err
	}
	updated, err := coreFunc(event)
	if err != nil {
		return emptyJSON, err
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
		return emptyJSON, fmt.Errorf("failed to marshal event data: %w", err)
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
		return emptyJSONArray, fmt.Errorf("failed to marshal event data: %w", err)
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
