package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/git-calendar/core/pkg/encryption"
	"github.com/go-git/go-billy/v5"
	"github.com/google/uuid"
)

// eventInFile represents event inside file. It doesn't have an Id and Calendar fields, since they can be derrived from the file path/name itself.
type eventInFile struct {
	Title       string      `json:"title,omitzero"`
	Location    string      `json:"location,omitzero"`
	Description string      `json:"description,omitzero"`
	From        time.Time   `json:"from,omitzero"`
	To          time.Time   `json:"to,omitzero"`
	Tag         uuid.UUID   `json:"tag,omitzero"`
	ParentId    uuid.UUID   `json:"parent_id,omitzero"`
	Repeat      *Repetition `json:"repeat,omitzero"`
	UpdatedAt   time.Time   `json:"updated_at,omitzero"`
}

func (ef eventInFile) toEvent(id uuid.UUID, calendar string) Event {
	return Event{
		Id:          id,
		Title:       ef.Title,
		Location:    ef.Location,
		Description: ef.Description,
		From:        ef.From,
		To:          ef.To,
		Calendar:    calendar,
		Tag:         ef.Tag,
		ParentId:    ef.ParentId,
		Repeat:      ef.Repeat,
		UpdatedAt:   ef.UpdatedAt,
	}
}

func (e Event) fileData() eventInFile {
	return eventInFile{
		Title:       e.Title,
		Location:    e.Location,
		Description: e.Description,
		From:        e.From,
		To:          e.To,
		Tag:         e.Tag,
		ParentId:    e.ParentId,
		Repeat:      e.Repeat,
		UpdatedAt:   e.UpdatedAt,
	}
}

func (e Event) WriteToFile(file billy.File, key []byte) error {
	if e.Id == uuid.Nil {
		return errors.New("event id has to be set")
	}

	// marshal normally
	raw, err := json.MarshalIndent(e.fileData(), "", "  ")
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
	encrypted, err := encryption.EncryptFields(plain, key, e.Id[:])
	if err != nil {
		return err
	}

	// marshal again
	finalRaw, err := json.MarshalIndent(encrypted, "", "  ")
	if err != nil {
		return err
	}

	_, err = file.Write(finalRaw)
	return err
}

func (e *Event) LoadFromFile(file billy.File, calendar string, decryptionKey []byte) error {
	raw, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", file.Name(), err)
	}

	return e.LoadFromBytes(raw, file.Name(), calendar, decryptionKey)
}

func (e *Event) LoadFromBytes(raw []byte, name string, calendar string, decryptionKey []byte) error {
	id, err := uuid.Parse(strings.TrimSuffix(path.Base(name), ".json")) // get event id from the name
	if err != nil {
		return err
	}

	var data eventInFile

	if len(decryptionKey) == 0 { // no encryption, just use the plaintext
		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}

		*e = data.toEvent(id, calendar)
		return nil
	}

	var encrypted map[string]any
	if err := json.Unmarshal(raw, &encrypted); err != nil {
		return err
	}

	decrypted, err := encryption.DecryptFields(encrypted, decryptionKey, id[:])
	if err != nil {
		return err
	}

	// eww (map to struct conversion)
	tmp, err := json.Marshal(decrypted)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(tmp, &data); err != nil {
		return err
	}

	*e = data.toEvent(id, calendar)
	return nil
}
