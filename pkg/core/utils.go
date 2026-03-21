package core

import (
	"encoding/binary"
	"errors"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/uuid"
)

func addUnit(t time.Time, value int, unit Freq) time.Time {
	switch unit {
	case Day:
		return t.AddDate(0, 0, value)
	case Week:
		return t.AddDate(0, 0, 7*value)
	case Month:
		return t.AddDate(0, value, 0)
	case Year:
		return t.AddDate(value, 0, 0)
	default:
		return t
	}
}

// Returns the first start time >= searchStart (or zero time if none reasonable).
// Also returns how many steps from the original (0 = original event time).
func firstOccurrenceAtOrAfter(searchStart time.Time, ev *Event) (time.Time, int) {
	if ev.Repeat == nil {
		if !searchStart.After(ev.From) {
			return ev.From, 0
		}
		return time.Time{}, -1 // none in range
	}

	current := ev.From
	steps := 0
	interval := max(1, ev.Repeat.Interval) // prevent crazy input

	const maxSteps = 36500 // safety limit (~100 years)

	for current.Before(searchStart) && steps < maxSteps {
		current = addUnit(current, interval, ev.Repeat.Frequency)
		steps++
	}

	if steps >= maxSteps || current.IsZero() {
		return time.Time{}, -1
	}

	return current, steps
}

func containsTime(exceptions []uuid.UUID, t time.Time) bool {
	for _, ex := range exceptions {
		exTime, err := getTimeFromUUID(ex)
		if err != nil {
			continue
		}
		if exTime.Equal(t) {
			return true
		}
	}
	return false
}

// Extracts the auth (http://USER:PASS@example.com/...) from repoUrl and returns a new url using proxyUrl if present.
func prepareRepoUrl(repoUrl url.URL, proxyUrl *url.URL) (url.URL, *http.BasicAuth) {
	// parse auth from url and delete the credentials
	auth := authFromUrl(repoUrl)
	repoUrl.User = nil

	// add proxy if specified
	if proxyUrl != nil {
		repoUrl = useCorsProxy(repoUrl, *proxyUrl)
	}

	return repoUrl, auth
}

// Merges the originalUrl with proxyUrl to use the cors proxy. Using the "url" query parameter.
//
// For Example:
//
//	originalUrl: "https://github.com/joe/my-calendar"
//	proxyUrl: "https://cors-proxy.abc"
//	out: "https://cors-proxy.abc/?url=https%3A%2F%2Fgithub.com%2Fjoe%2Fmy-calendar"
func useCorsProxy(originalUrl url.URL, proxyUrl url.URL) url.URL {
	// create the query parameter
	q := proxyUrl.Query()
	q.Set("url", originalUrl.String())

	// create the result with query param (e.g. https://cors-proxy.abc/?url=https://github.com/...)
	result := proxyUrl
	result.RawQuery = q.Encode()

	return result
}

// Extracts BasicAuth credentials from an URL.
func authFromUrl(u url.URL) *http.BasicAuth {
	credentials := u.User
	pass, ok := credentials.Password()
	if !ok && credentials.Username() == "" {
		return nil
	}

	return &http.BasicAuth{
		Username: credentials.Username(),
		Password: pass,
	}
}

// Turns "http://abc.com/foo/bar/my-calendar.git" into "my-calendar".
func calendarNameFromUrl(u url.URL) string {
	name := path.Base(u.Path)
	if name == "." || name == "/" {
		return "shouldnthappen"
	}
	return strings.TrimSuffix(name, ".git")
}

// Generates custom uuid from masterId and some time. It uses 6 bytes for the master and 6 bytes for the time
// If the generation fails, it returns uuid.New()
func generateCustomUUID(masterId uuid.UUID, t time.Time) uuid.UUID {
	idBuf := make([]byte, 16)
	copy(idBuf[:6], masterId[:6])      // take first 6 bytes from masterId
	copy(idBuf[9:12], masterId[13:16]) // take another 3 bytes from masterId
	idBuf[6] = 0x80                    // set version
	idBuf[7] = 0x69                    // could be a flag, but now is just 0x69
	idBuf[8] = 0x80                    // RFC 9562
	unix32 := uint32(t.Unix())
	binary.BigEndian.PutUint32(idBuf[12:16], unix32) // add the time
	id, err := uuid.FromBytes(idBuf)
	if err != nil {
		return uuid.New()
	}
	return id
}

// extracts time from custom UUIDv8
func getTimeFromUUID(id uuid.UUID) (time.Time, error) {
	// check if the id is v8
	if id[6] != 0x80 {
		return time.Time{}, errors.New("invalid UUID")
	}
	unix32 := binary.BigEndian.Uint32(id[12:16])
	return time.Unix(int64(unix32), 0), nil
}

// takes custom UUISv8 and shifts the time by duration
func getShiftedUUID(id uuid.UUID, duration time.Duration) uuid.UUID {
	idBuf := make([]byte, 16)
	copy(idBuf[0:16], id[:8])
	if idBuf[6] != 0x80 {
		return uuid.Nil
	}
	shiftedTime := uint32(time.Unix(int64(binary.BigEndian.Uint32(id[12:16])), 0).Add(duration).Unix())
	binary.BigEndian.PutUint32(idBuf[12:16], shiftedTime) // add the time
	newId, err := uuid.FromBytes(idBuf)
	if err != nil {
		return uuid.Nil
	}
	return newId
}

// Combines the base date with source clock/time only. For example:
//
//	base:   2026-03-11T10:30:00
//	source: 2025-01-01T09:00:00
//	output: 2026-03-11T09:00:00
func withTimeOfDay(base, clockSource time.Time) time.Time {
	return time.Date(
		base.Year(), base.Month(), base.Day(),
		clockSource.Hour(), clockSource.Minute(), clockSource.Second(), clockSource.Nanosecond(),
		base.Location(),
	)
}
