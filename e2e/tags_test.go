package e2e

import (
	"testing"

	"github.com/git-calendar/core/pkg/core"
	"github.com/google/uuid"
)

func TestTag_CreateTag_CreatesTag(t *testing.T) {
	c := newTestCore(t)

	tag := newTestTag("work", "blue")

	created, err := c.CreateTag(testCalendarName, tag)
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}
	if created == nil {
		t.Fatalf("expected created tag")
	}

	assertTagEqual(t, tag, *created)
}

func TestTag_CreateTag_DuplicateReturnsError(t *testing.T) {
	c := newTestCore(t)

	tag := newTestTag("work", "blue")

	if _, err := c.CreateTag(testCalendarName, tag); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	created, err := c.CreateTag(testCalendarName, tag)
	if err == nil {
		t.Fatalf("expected duplicate tag error")
	}
	if created != nil {
		t.Fatalf("expected nil created tag, got %+v", created)
	}
}

func TestTag_CreateTag_InvalidTagReturnsError(t *testing.T) {
	c := newTestCore(t)

	tag := core.Tag{
		ID:    uuid.New(),
		Color: "blue",
	}

	created, err := c.CreateTag(testCalendarName, tag)
	if err == nil {
		t.Fatalf("expected invalid tag error")
	}
	if created != nil {
		t.Fatalf("expected nil tag on error, got %#v", created)
	}
}

func TestTag_CreateTag_ReadonlyCalendarReturnsError(t *testing.T) {
	c := newTestCore(t)
	if err := c.UpdateRemote(testCalendarName, mustParseURL("https://example.com/calendar.git"), true); err != nil {
		t.Fatalf("failed to make calendar read-only: %v", err)
	}

	created, err := c.CreateTag(testCalendarName, newTestTag("work", "blue"))
	if err == nil {
		t.Fatal("expected read-only calendar error")
	}
	if created != nil {
		t.Fatalf("expected nil tag on error, got %#v", created)
	}
}

func TestTag_UpdateTag_UpdatesExistingTag(t *testing.T) {
	c := newTestCore(t)

	tag := newTestTag("work", "blue")

	if _, err := c.CreateTag(testCalendarName, tag); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	updated := core.Tag{
		ID:    tag.ID,
		Name:  "personal",
		Color: "blue",
	}

	got, err := c.UpdateTag(testCalendarName, updated)
	if err != nil {
		t.Fatalf("UpdateTag failed: %v", err)
	}
	if got == nil {
		t.Fatalf("expected updated tag")
	}

	assertTagEqual(t, updated, *got)
}

func TestTag_UpdateTag_InvalidTagReturnsError(t *testing.T) {
	c := newTestCore(t)

	tag := core.Tag{
		Name:  "work",
		Color: "red",
	}

	updated, err := c.UpdateTag(testCalendarName, tag)
	if err == nil {
		t.Fatalf("expected invalid tag error")
	}
	if updated != nil {
		t.Fatalf("expected nil updated tag, got %+v", updated)
	}
}

func TestTag_UpdateTag_MissingTagReturnsErrorInsteadOfPanicking(t *testing.T) {
	c := newTestCore(t)

	tag := newTestTag("missing", "blue")

	updated, err := c.UpdateTag(testCalendarName, tag)
	if err == nil {
		t.Fatalf("expected missing tag error")
	}
	if updated != nil {
		t.Fatalf("expected nil updated tag, got %+v", updated)
	}
}

func TestTag_RemoveTag_RemovesTag(t *testing.T) {
	c := newTestCore(t)

	tag := newTestTag("work", "blue")

	if _, err := c.CreateTag(testCalendarName, tag); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	if err := c.RemoveTag(testCalendarName, tag.ID); err != nil {
		t.Fatalf("RemoveTag failed: %v", err)
	}

	// creating the same id again proves it was removed from the in-memory tag list
	recreated := core.Tag{
		ID:    tag.ID,
		Name:  "recreated",
		Color: "blue",
	}

	got, err := c.CreateTag(testCalendarName, recreated)
	if err != nil {
		t.Fatalf("CreateTag after RemoveTag failed: %v", err)
	}
	if got == nil {
		t.Fatalf("expected recreated tag")
	}

	assertTagEqual(t, recreated, *got)
}

func TestTag_RemoveTag_InvalidIDReturnsError(t *testing.T) {
	c := newTestCore(t)

	err := c.RemoveTag(testCalendarName, uuid.Nil)
	if err == nil {
		t.Fatalf("expected invalid tag id error")
	}
}

func TestTag_InvalidCalendarReturnsError(t *testing.T) {
	c := newTestCore(t)

	tag := newTestTag("work", "green")

	if _, err := c.CreateTag("missing", tag); err == nil {
		t.Fatalf("expected CreateTag invalid calendar error")
	}

	if _, err := c.UpdateTag("missing", tag); err == nil {
		t.Fatalf("expected UpdateTag invalid calendar error")
	}

	if err := c.RemoveTag("missing", tag.ID); err == nil {
		t.Fatalf("expected RemoveTag invalid calendar error")
	}
}

func newTestTag(name string, color string) core.Tag {
	return core.Tag{
		ID:    uuid.New(),
		Name:  name,
		Color: color,
	}
}

func assertTagEqual(t *testing.T, want core.Tag, got core.Tag) {
	t.Helper()

	if got.ID != want.ID {
		t.Fatalf("tag id mismatch: expected %s, got %s", want.ID, got.ID)
	}
	if got.Name != want.Name {
		t.Fatalf("tag name mismatch: expected %q, got %q", want.Name, got.Name)
	}
	if got.Color != want.Color {
		t.Fatalf("tag color mismatch: expected %q, got %q", want.Color, got.Color)
	}
}
