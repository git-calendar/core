package core

import "testing"

func TestListCalendarsSortsClonedTags(t *testing.T) {
	work := &Calendar{
		Name: "Work",
		Tags: []Tag{
			{Name: "Zulu"},
			{Name: "Alpha"},
		},
		EncryptionKey: []byte{1},
	}
	c := &Core{
		calendars: map[string]*Calendar{
			"Work": work,
			"Home": {Name: "Home"},
		},
	}

	calendars, err := c.ListCalendars()
	if err != nil {
		t.Fatal(err)
	}
	if len(calendars) != 2 {
		t.Fatalf("got %d calendars, want 2", len(calendars))
	}
	if calendars[0].Name != "Home" || calendars[1].Name != "Work" {
		t.Fatalf("calendar order = [%q, %q], want [Home, Work]", calendars[0].Name, calendars[1].Name)
	}
	if calendars[1].Tags[0].Name != "Alpha" || calendars[1].Tags[1].Name != "Zulu" {
		t.Fatalf("tag order = [%q, %q], want [Alpha, Zulu]", calendars[1].Tags[0].Name, calendars[1].Tags[1].Name)
	}
	if work.Tags[0].Name != "Zulu" || work.Tags[1].Name != "Alpha" {
		t.Fatalf("stored tag order changed to [%q, %q]", work.Tags[0].Name, work.Tags[1].Name)
	}

	calendars[1].Tags[0].Name = "Changed"
	calendars[1].EncryptionKey[0] = 2
	if work.Tags[1].Name != "Alpha" {
		t.Errorf("returned tags alias stored tags: stored name = %q", work.Tags[1].Name)
	}
	if work.EncryptionKey[0] != 1 {
		t.Errorf("returned key aliases stored key: stored key = %d", work.EncryptionKey[0])
	}
}
