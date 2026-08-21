// Package errcode adds stable, machine-readable codes to errors.
package errcode

import "errors"

// Code identifies an error category across API boundaries.
type Code string

const (
	Network    Code = "NETWORK"
	Auth       Code = "AUTH"
	Forbidden  Code = "FORBIDDEN"
	Validation Code = "VALIDATION"
	Storage    Code = "STORAGE"
)

type codedError struct {
	code Code
	err  error
}

type calendarError struct {
	calendar string
	err      error
}

// Wrap associates err with code. It returns nil when err is nil.
func Wrap(code Code, err error) error {
	if err == nil {
		return nil
	}
	return &codedError{code: code, err: err}
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// WithCalendar associates err with a calendar. It returns nil when err is nil.
func WithCalendar(calendar string, err error) error {
	if err == nil {
		return nil
	}
	if calendar == "" {
		return err
	}
	return &calendarError{calendar: calendar, err: err}
}

func (e *calendarError) Error() string { return e.err.Error() }
func (e *calendarError) Unwrap() error { return e.err }

// CodeOf returns the first error code in err's chain.
func CodeOf(err error) (Code, bool) {
	var coded *codedError
	if !errors.As(err, &coded) {
		return "", false
	}
	return coded.code, true
}

// CalendarOf returns a calendar when all calendar-specific errors refer to the same one.
func CalendarOf(err error) (string, bool) {
	calendars := make(map[string]struct{})
	collectCalendars(err, calendars)
	if len(calendars) != 1 {
		return "", false
	}
	for calendar := range calendars {
		return calendar, true
	}
	return "", false
}

func collectCalendars(err error, calendars map[string]struct{}) {
	if err == nil {
		return
	}
	if calendarErr, ok := err.(*calendarError); ok {
		calendars[calendarErr.calendar] = struct{}{}
	}

	switch err := err.(type) {
	case interface{ Unwrap() []error }:
		for _, child := range err.Unwrap() {
			collectCalendars(child, calendars)
		}
	case interface{ Unwrap() error }:
		collectCalendars(err.Unwrap(), calendars)
	}
}

var (
	_ error = (*codedError)(nil)
	_ error = (*calendarError)(nil)
)
