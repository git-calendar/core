package errcode

import (
	"errors"
	"fmt"
	"testing"
)

func TestWrap(t *testing.T) {
	target := errors.New("offline")
	err := Wrap(Network, target)

	if !errors.Is(err, target) {
		t.Fatal("wrapped error does not preserve its cause")
	}
	if got, ok := CodeOf(fmt.Errorf("sync failed: %w", err)); !ok || got != Network {
		t.Fatalf("CodeOf() = %q, %t, want %q, true", got, ok, Network)
	}
	if got := err.Error(); got != target.Error() {
		t.Fatalf("Error() = %q, want %q", got, target.Error())
	}
}

func TestWrapNil(t *testing.T) {
	if err := Wrap(Network, nil); err != nil {
		t.Fatalf("Wrap() = %v, want nil", err)
	}
}

func TestCodeOfUncategorizedError(t *testing.T) {
	if got, ok := CodeOf(errors.New("unexpected")); ok {
		t.Fatalf("CodeOf() = %q, true, want no code", got)
	}
}

func TestCodeOfJoinedErrors(t *testing.T) {
	err := errors.Join(
		errors.New("unexpected"),
		Wrap(Storage, errors.New("database unavailable")),
	)

	if got, ok := CodeOf(err); !ok || got != Storage {
		t.Fatalf("CodeOf() = %q, %t, want %q, true", got, ok, Storage)
	}
}

func TestWithCalendar(t *testing.T) {
	target := errors.New("offline")
	err := WithCalendar("work", fmt.Errorf("sync failed: %w", target))

	if !errors.Is(err, target) {
		t.Fatal("wrapped error does not preserve its cause")
	}
	if got, ok := CalendarOf(err); !ok || got != "work" {
		t.Fatalf("CalendarOf() = %q, %t, want %q, true", got, ok, "work")
	}
}

func TestWithCalendarNil(t *testing.T) {
	if err := WithCalendar("work", nil); err != nil {
		t.Fatalf("WithCalendar() = %v, want nil", err)
	}
}

func TestCalendarOfJoinedErrors(t *testing.T) {
	t.Run("same calendar", func(t *testing.T) {
		err := errors.Join(
			WithCalendar("work", errors.New("fetch failed")),
			WithCalendar("work", errors.New("push failed")),
		)

		if got, ok := CalendarOf(err); !ok || got != "work" {
			t.Fatalf("CalendarOf() = %q, %t, want %q, true", got, ok, "work")
		}
	})

	t.Run("different calendars", func(t *testing.T) {
		err := errors.Join(
			WithCalendar("work", errors.New("fetch failed")),
			WithCalendar("personal", errors.New("fetch failed")),
		)

		if got, ok := CalendarOf(err); ok {
			t.Fatalf("CalendarOf() = %q, true, want no calendar", got)
		}
	})
}
