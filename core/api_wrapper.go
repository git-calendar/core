package core

import (
	"encoding/json"
	"fmt"
)

type (
	// The exposed/exported API interface
	//
	// It's not possible to expose any "complex" types (structs*, arrays, channels, maps, etc.), because they do not have bindings to other languages.
	//
	// Let's use JSON everywhere as a REST API would... (or protobuf?)
	//
	// * You can return a *Event (pointer to struct), but you cannot receive it as argument.
	JsonApi interface {
		Initialize() error
		Clone(repoUrl string) error
		// AddRemote()
		// Delete()
		SetCorsProxy(proxyUrl string) error

		AddEvent(eventJson string) error
		UpdateEvent(eventJson string) error
		RemoveEvent(eventJson string) error
		GetEvent(id int) (string, error)
		GetEvents(from int64, to int64) (string, error)
	}

	jsonApiImpl struct {
		inner Api
	}
)

func NewJsonApi() JsonApi {
	return &jsonApiImpl{
		inner: NewApi(),
	}
}

func (a *jsonApiImpl) Initialize() error {
	return a.inner.Initialize()
}

func (a *jsonApiImpl) Clone(repoUrl string) error {
	return a.inner.Clone(repoUrl)
}

func (a *jsonApiImpl) SetCorsProxy(proxyUrl string) error {
	return a.inner.SetCorsProxy(proxyUrl)
}

func (a *jsonApiImpl) AddEvent(eventJson string) error {
	event, err := parseAndValidateEventHelper(eventJson)
	if err != nil {
		return err
	}
	return a.inner.AddEvent(event)
}

func (a *jsonApiImpl) UpdateEvent(eventJson string) error {
	event, err := parseAndValidateEventHelper(eventJson)
	if err != nil {
		return err
	}

	return a.inner.UpdateEvent(event)
}

func (a *jsonApiImpl) RemoveEvent(eventJson string) error {
	event, err := parseAndValidateEventHelper(eventJson)
	if err != nil {
		return err
	}

	return a.inner.RemoveEvent(event)
}

func (a *jsonApiImpl) GetEvent(id int) (string, error) {
	// pass the id to inner api
	event, err := a.inner.GetEvent(id)
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

func (a *jsonApiImpl) GetEvents(from int64, to int64) (string, error) {
	// pass the args to inner api
	events, err := a.inner.GetEvents(from, to)
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

// ------------------------------ helpers ------------------------------

func parseAndValidateEventHelper(eventJson string) (Event, error) {
	// unmarshal to struct
	var e Event
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
