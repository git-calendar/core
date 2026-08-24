package core

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
	"uuid"

	"github.com/git-calendar/core/pkg/encryption"
	"github.com/go-git/go-billy/v5"
	rrule "github.com/teambition/rrule-go"
)

// eventInFile represents event inside file.
// It doesn't have an ID and Calendar fields, since they can be derrived from the file path/name itself.
type eventInFile struct {
	Title       string     `json:"title,omitzero"`
	Location    string     `json:"location,omitzero"`
	Description string     `json:"description,omitzero"`
	From        time.Time  `json:"from,omitzero"`
	To          time.Time  `json:"to,omitzero"`
	TagID       *uuid.UUID `json:"tag_id,omitzero"`
	ParentID    *uuid.UUID `json:"parent_id,omitzero"`
	Repeat      string     `json:"repeat,omitzero"`
	UpdatedAt   time.Time  `json:"updated_at,omitzero"`
}

func (ef eventInFile) toEvent(id uuid.UUID, calendar string) (Event, error) {
	var repeat *rrule.Set
	if ef.Repeat != "" {
		var err error
		repeat, err = rrule.StrToRRuleSet(ef.Repeat)
		if err != nil {
			return Event{}, fmt.Errorf("invalid recurrence: %w", err)
		}
	}
	return Event{
		ID:          id,
		Title:       ef.Title,
		Location:    ef.Location,
		Description: ef.Description,
		From:        ef.From,
		To:          ef.To,
		Calendar:    calendar,
		TagID:       ef.TagID,
		ParentID:    ef.ParentID,
		Repeat:      repeat,
		UpdatedAt:   ef.UpdatedAt,
	}, nil
}

func (e Event) fileData() eventInFile {
	var repeat string
	if e.Repeat != nil {
		repeat = e.Repeat.String()
	}
	return eventInFile{
		Title:       e.Title,
		Location:    e.Location,
		Description: e.Description,
		From:        e.From,
		To:          e.To,
		TagID:       e.TagID,
		ParentID:    e.ParentID,
		Repeat:      repeat,
		UpdatedAt:   e.UpdatedAt,
	}
}

// WriteToFile serializes the event to a repository file, encrypting it when key is set.
func (e Event) WriteToFile(file billy.File, key []byte) error {
	if e.ID == uuid.Nil() {
		return errors.New("event id has to be set")
	}

	// marshal normally
	raw, err := json.Marshal(e.fileData(), json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return err
	}

	if len(key) == 0 { // no encryption, just use the plaintext
		_, err = file.Write(raw)
		return err
	}

	// unmarshal into generic map
	var plain map[string]any
	if err := json.Unmarshal(raw, &plain); err != nil {
		return err
	}

	// encrypt everything recursively
	encrypted, err := encryption.EncryptFields(plain, key, e.ID[:])
	if err != nil {
		return err
	}

	// marshal again
	finalRaw, err := json.Marshal(encrypted, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return err
	}

	_, err = file.Write(finalRaw)
	return err
}

// LoadFromFile decodes an event from a repository file.
func (e *Event) LoadFromFile(file billy.File, calendar string, decryptionKey []byte) error {
	raw, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", file.Name(), err)
	}

	return e.LoadFromBytes(raw, file.Name(), calendar, decryptionKey)
}

// LoadFromBytes decodes an event, deriving its ID from the supplied file name.
func (e *Event) LoadFromBytes(raw []byte, name string, calendar string, decryptionKey []byte) error {
	id, err := uuid.Parse(strings.TrimSuffix(path.Base(name), ".json"))
	if err != nil {
		return err
	}

	if len(decryptionKey) != 0 {
		var encrypted map[string]any
		if err := json.Unmarshal(raw, &encrypted); err != nil {
			return err
		}
		decrypted, err := encryption.DecryptFields(encrypted, decryptionKey, id[:])
		if err != nil {
			return err
		}
		raw, err = json.Marshal(decrypted, json.Deterministic(true))
		if err != nil {
			return err
		}
	}

	var data eventInFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	loaded, err := data.toEvent(id, calendar)
	if err != nil {
		return err
	}
	*e = loaded
	return nil
}
