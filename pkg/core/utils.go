package core

import (
	"encoding/binary"
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

// firstOccurrenceAtOrAfter returns the first start time >= searchStart (or zero time if none reasonable).
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
	const maxSteps = 36500 // safety limit (~100 years for freq=Daily)

	for current.Before(searchStart) && steps < maxSteps {
		current = addUnit(current, ev.Repeat.Interval, ev.Repeat.Frequency)
		steps++
	}

	if current.IsZero() || steps >= maxSteps {
		return time.Time{}, -1
	}

	return current, steps
}

func containsTime(exceptions []uuid.UUID, t time.Time) bool {
	for _, ex := range exceptions {
		exTime := getTimeFromUUID(ex)
		if exTime.IsZero() {
			continue
		}
		if exTime.Equal(t) {
			return true
		}
	}
	return false
}

// prepareRepoUrl extracts the auth (http://USER:PASS@example.com/...) from repoUrl and returns a new url using proxyUrl if present.
func prepareRepoUrl(repoUrl *url.URL, proxyUrl *url.URL) (*url.URL, *http.BasicAuth) {
	if repoUrl == nil {
		return nil, nil
	}
	repo := *repoUrl // copy

	// parse auth from url and delete the credentials
	auth := authFromUrl(&repo)
	repo.User = nil

	// add proxy if specified
	if proxyUrl != nil {
		return useCorsProxy(&repo, proxyUrl), auth
	}

	return &repo, auth
}

// useCorsProxy returns a new URL that routes the original URL through the given CORS proxy.
// The full original URL (including scheme) is appended as the path to the proxy.
func useCorsProxy(original, proxy *url.URL) *url.URL {
	if proxy == nil {
		return original
	}
	if original == nil {
		u := *proxy
		return &u
	}

	p := *proxy // copy

	// cut trailing slash
	base := strings.TrimRight(p.String(), "/")
	if base == "" {
		base = p.Scheme + "://" + p.Host
	}
	result, err := url.Parse(base + "/" + original.String())
	if err != nil {
		return nil
	}

	return result
}

// authFromUrl extracts BasicAuth credentials from an URL.
func authFromUrl(u *url.URL) *http.BasicAuth {
	if u == nil {
		return nil
	}

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

// calendarNameFromUrl turns "http://abc.com/foo/bar/my-calendar.git" into "my-calendar".
func calendarNameFromUrl(u *url.URL) string {
	if u == nil {
		return ""
	}

	name := path.Base(u.Path)
	if name == "." || name == "/" {
		return "shouldnthappen"
	}
	return strings.TrimSuffix(name, ".git")
}

// generateCustomUUID generates custom uuid from parentId and some time. It uses 6 bytes for the parent and 6 bytes for the time.
// If the generation fails, it returns uuid.New().
func generateCustomUUID(parentId uuid.UUID, t time.Time) uuid.UUID {
	idBuf := make([]byte, 16)
	copy(idBuf[:6], parentId[:6])      // take first 6 bytes from parentId
	copy(idBuf[9:12], parentId[13:16]) // take another 3 bytes from parentId
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

// getTimeFromUUID extracts time from custom UUIDv8.
func getTimeFromUUID(id uuid.UUID) time.Time {
	if id.Version() != 8 {
		return time.Time{}
	}
	unix32 := binary.BigEndian.Uint32(id[12:16])
	return time.Unix(int64(unix32), 0)
}

// getShiftedUUID returns a copy of a UUIDv8 with its custom 32-bit timestamp (stored in bytes 12–15, big-endian) shifted by the given duration.
// Returns uuid.Nil if the input id is not v8.
func getShiftedUUID(id uuid.UUID, duration time.Duration) uuid.UUID {
	if id.Version() != 8 {
		return uuid.Nil
	}

	// extract original timestamp (big-endian bytes 12-15)
	origTime := binary.BigEndian.Uint32(id[12:16])

	// calculate shift in whole seconds
	secondsShift := int64(duration.Seconds())
	shiftedTime := uint32(int64(origTime) + secondsShift)

	// create new UUID and write back
	newId := id // UUID is [16]byte, so this is a value copy
	binary.BigEndian.PutUint32(newId[12:16], shiftedTime)

	return newId
}

// splitExceptions returns two exceptions groups. One with exceptions before and one with exceptions after the specified cutoff.
func splitExceptions(exceptions []uuid.UUID, cutoff time.Time) (before, after []uuid.UUID) {
	for _, ex := range exceptions {
		if getTimeFromUUID(ex).Before(cutoff) {
			before = append(before, ex)
		} else {
			after = append(after, ex)
		}
	}
	return
}
