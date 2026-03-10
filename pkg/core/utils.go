package core

import (
	"net/url"
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

func getFirstCandidate(searchStart time.Time, event *Event) (time.Time, int) {
	switch event.Repeat.Frequency {
	case Day:
		diffHours := searchStart.Sub(event.From).Hours()
		cycleHours := 24.0 * float64(event.Repeat.Interval)
		cycles := int(diffHours / cycleHours)
		days := cycles * event.Repeat.Interval
		return addUnit(event.From, days, Day), cycles
	case Week:
		diffHours := searchStart.Sub(event.From).Hours()
		cycleHours := 24.0 * 7 * float64(event.Repeat.Interval)
		cycles := int(diffHours / cycleHours)
		weeks := cycles * event.Repeat.Interval
		return addUnit(event.From, weeks, Week), cycles
	case Month:
		diffMonths := (searchStart.Year()-event.From.Year())*12 + int(searchStart.Month()-event.From.Month())
		cycles := diffMonths / event.Repeat.Interval
		months := cycles * event.Repeat.Interval
		candidate := addUnit(event.From, months, Month)
		if candidate.Before(searchStart) {
			candidate = addUnit(event.From, event.Repeat.Interval, Month)
			cycles++
		}
		return candidate, cycles
	case Year:
		diffYears := searchStart.Year() - event.From.Year()
		cycles := diffYears / event.Repeat.Interval
		years := cycles * event.Repeat.Interval
		candidate := addUnit(event.From, years, Year)
		if candidate.Before(searchStart) {
			candidate = addUnit(event.From, event.Repeat.Interval, Year)
			cycles++
		}
		return candidate, cycles
	default:
		return event.From, -1
	}
}

func containsId(exceptions []Exception, id uuid.UUID) bool {
	for _, ex := range exceptions {
		if ex.Id == id {
			return true
		}
	}
	return false
}

func containsTime(exceptions []Exception, t time.Time) bool {
	for _, ex := range exceptions {
		if ex.Time.Equal(t) {
			return true
		}
	}
	return false
}

func isGeneratedEvent(event Event) bool {
	return event.MasterId != uuid.Nil
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
	pass, _ := credentials.Password()

	return &http.BasicAuth{
		Username: credentials.Username(),
		Password: pass,
	}
}

func calendarNameFromUrl(u url.URL) string {
	segments := strings.Split(u.Path, "/")
	if len(segments) == 0 {
		return ""
	}
	name := segments[len(segments)-1]
	name = strings.TrimSuffix(name, ".git")
	return name
}
