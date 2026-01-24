// A JSON core API wrapper for multiplatform support.
// It's not possible to expose any "complex" data types (structs*, arrays, channels, maps, etc.),
// because they do not have bindings to other languages.
// Let's use JSON everywhere as a REST API would... (or protobuf?)
//
// (*) You can return a *Event (pointer to struct), but you cannot receive it as argument.
package gocore

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/firu11/git-calendar-core/pkg/core"
	"github.com/google/uuid"
)

type (
	// The exposed/exported JSON-only API interface
	Api interface {
		core.BaseApi

		AddEvent(eventJson string) error
		UpdateEvent(eventJson string) error
		RemoveEvent(eventJson string) error
		GetEvent(id string) (string, error)
		GetEvents(from, to int64) (string, error)
	}

	// Private implementation of Api
	apiImpl struct {
		inner core.CoreApi
	}
)

// A "constructor" for JsonApi.
func NewApi() Api {
	return &apiImpl{
		inner: core.NewCoreApi(),
	}
}

// -------------------------- Boring methods to satisfy the core.BaseApi interface --------------------------
func (a *apiImpl) Initialize() error                      { return a.inner.Initialize() }
func (a *apiImpl) Clone(repoUrl string) error             { return a.inner.Clone(repoUrl) }
func (a *apiImpl) AddRemote(name, remoteUrl string) error { return a.inner.AddRemote(name, remoteUrl) }
func (a *apiImpl) Delete() error                          { return a.inner.Delete() }
func (a *apiImpl) SetCorsProxy(proxyUrl string) error     { return a.inner.SetCorsProxy(proxyUrl) }

// ------------------------------  Wrapper methods encoding and decoding JSONs ------------------------------
func (a *apiImpl) AddEvent(eventJson string) error {
	event, err := parseAndValidateEventHelper(eventJson)
	if err != nil {
		return err
	}
	return a.inner.AddEvent(event)
}

func (a *apiImpl) UpdateEvent(eventJson string) error {
	event, err := parseAndValidateEventHelper(eventJson)
	if err != nil {
		return err
	}

	return a.inner.UpdateEvent(event)
}

func (a *apiImpl) RemoveEvent(eventJson string) error {
	event, err := parseAndValidateEventHelper(eventJson)
	if err != nil {
		return err
	}

	return a.inner.RemoveEvent(event)
}

func (a *apiImpl) GetEvent(id string) (string, error) {
	parsedId, err := uuid.Parse(id)
	if err != nil {
		return "", fmt.Errorf("invalid event id: %w", err)
	}
	// pass the id to inner api
	event, err := a.inner.GetEvent(parsedId)
	if err != nil {
		return "", err
	}

	// marshal to json
	jsonBytes, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("failed to marshal event to json (this should not happen, please open an Issue): %w", err)
	}

	return string(jsonBytes), nil
}

func (a *apiImpl) GetEvents(from, to int64) (string, error) {
	// pass the args to inner api
	events, err := a.inner.GetEvents(minutesToTime(from), minutesToTime(to))
	if err != nil {
		return "", err
	}

	// marshal to json
	jsonBytes, err := json.Marshal(events)
	if err != nil {
		return "", fmt.Errorf("failed to marshal events to json (this should not happen, please open an Issue): %w", err)
	}

	return string(jsonBytes), nil
}

// ------------------------------------------------ Helpers -------------------------------------------------

// Unmarshalls event from JSON to core.Event for internal use and validates the input.
func parseAndValidateEventHelper(eventJson string) (core.Event, error) {
	// unmarshal to struct
	var e core.Event
	err := json.Unmarshal([]byte(eventJson), &e)
	if err != nil {
		return e, fmt.Errorf("failed to parse event data: %w", err)
	}
	// validate
	err = e.Validate()
	if err != nil {
		return e, fmt.Errorf("invalid event data: %w", err)
	}

	return e, nil
}

// Converts a Unix timestamp expressed in minutes to time.Time (should be in UTC timezone).
func minutesToTime(unixMinutes int64) time.Time {
	// 1 minute = 60 seconds
	seconds := unixMinutes * 60
	return time.Unix(seconds, 0).UTC()
}
