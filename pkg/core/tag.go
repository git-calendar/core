package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/git-calendar/core/pkg/encryption"
	"github.com/go-git/go-billy/v5"
	"github.com/google/uuid"
)

type Tag struct {
	Id    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
}

func (t *Tag) Validate() error {
	if t == nil {
		return nil
	}
	if t.Id != uuid.Nil {
		// if id is set
		if t.Id.Version() != 4 && t.Id.Version() != 8 { // enforce version
			return errors.New("unsupported UUID version")
		}
	} else { // if id is unset
		t.Id = uuid.New() // create one if not specified
	}
	if t.Name == "" {
		return errors.New("tag name cannot be empty")
	}
	if t.Color == "" {
		return errors.New("tag color cannot be empty")
	}
	return nil
}

func (tag Tag) getPath() string {
	return path.Join(TagsDirName, tag.Id.String()+".json")
}

// ----------------------------------------------------------------

// tagFile represents a tag inside file.
type tagInFile struct {
	Name  string `json:"name,omitzero"`
	Color string `json:"color,omitzero"`
}

func (tf tagInFile) toTag(id uuid.UUID) Tag {
	return Tag{
		Id:    id,
		Name:  tf.Name,
		Color: tf.Color,
	}
}

func (e Tag) fileData() tagInFile {
	return tagInFile{
		Name:  e.Name,
		Color: e.Color,
	}
}

func (e Tag) WriteToFile(file billy.File, key []byte) error {
	if e.Id == uuid.Nil {
		return errors.New("tag id has to be set")
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

func (e *Tag) LoadFromFile(file billy.File, decryptionKey []byte) error {
	raw, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", file.Name(), err)
	}

	return e.LoadFromBytes(raw, file.Name(), decryptionKey)
}

func (e *Tag) LoadFromBytes(raw []byte, filename string, decryptionKey []byte) error {
	id, err := uuid.Parse(strings.TrimSuffix(path.Base(filename), ".json")) // get tag id from the filename
	if err != nil {
		return err
	}

	var data tagInFile

	if len(decryptionKey) == 0 { // no encryption, just use the plaintext
		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}

		*e = data.toTag(id)
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

	*e = data.toTag(id)
	return nil
}
