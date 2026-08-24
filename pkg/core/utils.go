package core

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"
	"uuid"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// prepareRepoURL separates HTTP credentials and optionally routes through a browser CORS proxy as <proxy>/<repository-url>.
func prepareRepoURL(repoURL *url.URL, proxyURL *url.URL) (*url.URL, *http.BasicAuth) {
	if repoURL == nil {
		return nil, nil
	}
	repo := repoURL.Clone()

	// parse auth from url and delete the credentials
	auth := authFromURL(repo)
	repo.User = nil

	// add proxy if specified
	if proxyURL != nil {
		return useCorsProxy(repo, proxyURL), auth
	}

	return repo, auth
}

func repoURLFromCalendar(cal *Calendar) (*url.URL, error) {
	remote, err := cal.repository.Remote(GitRemoteName)
	if err != nil {
		if errors.Is(err, gogit.ErrRemoteNotFound) {
			return nil, err // this is ok
		}
		return nil, fmt.Errorf("%q: failed to get remote: %w", cal.Name, err)
	}

	cfg := remote.Config()
	if cfg == nil {
		return nil, fmt.Errorf("%q: remote has no config", cal.Name)
	}
	if len(cfg.URLs) != 1 || cfg.URLs[0] == "" {
		return nil, fmt.Errorf("%q: remote must have exactly one non-empty URL", cal.Name)
	}
	repoURL, err := url.Parse(cfg.URLs[0])
	if err != nil {
		return nil, fmt.Errorf("%q: failed to parse remote URL: %w", cal.Name, err)
	}

	return repoURL, nil
}

// useCorsProxy returns a new URL that routes the original URL through the given CORS proxy.
// The full original URL (including scheme) is appended as the path to the proxy.
func useCorsProxy(original, proxy *url.URL) *url.URL {
	if proxy == nil {
		return original
	}
	if original == nil {
		return proxy.Clone()
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

// authFromURL extracts BasicAuth credentials from a URL.
func authFromURL(u *url.URL) *http.BasicAuth {
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

// calendarNameFromURL derives a name from a repository URL;
// for example, "https://example.com/foo/my-calendar.git" returns "my-calendar".
func calendarNameFromURL(u *url.URL) string {
	if u == nil {
		return ""
	}

	name := path.Base(u.Path)
	if name == "." || name == "/" {
		return "shouldnthappen"
	}
	return strings.TrimSuffix(name, ".git")
}

func uuidVersion(id uuid.UUID) byte {
	return id[6] >> 4
}

// newUUIDv5 generates a deterministic UUID using RFC 9562's SHA-1 algorithm.
func newUUIDv5(namespace uuid.UUID, data []byte) uuid.UUID {
	hash := sha1.New()
	hash.Write(namespace[:])
	hash.Write(data)

	var id uuid.UUID
	copy(id[:], hash.Sum(nil))
	id[6] = (id[6] & 0x0f) | 0x50
	id[8] = (id[8] & 0x3f) | 0x80
	return id
}

// generateCustomUUID generates a UUIDv8 from a parent ID and time.
func generateCustomUUID(parentID uuid.UUID, t time.Time) uuid.UUID {
	var id uuid.UUID
	copy(id[:6], parentID[:6])      // take first 6 bytes from parentID
	copy(id[9:12], parentID[13:16]) // take another 3 bytes from parentID
	id[6] = 0x80                    // set version
	id[7] = 0x69                    // could be a flag, but now is just 0x69
	id[8] = 0x80                    // RFC 9562
	binary.BigEndian.PutUint32(id[12:16], uint32(t.Unix()))
	return id
}

// getTimeFromUUID extracts the encoded time from a custom UUIDv8.
func getTimeFromUUID(id uuid.UUID) time.Time {
	if uuidVersion(id) != 8 {
		return time.Time{}
	}
	unix32 := binary.BigEndian.Uint32(id[12:16])
	return time.Unix(int64(unix32), 0)
}

// CalendarEventsFilter controls event visibility for one calendar.
type CalendarEventsFilter struct {
	// HiddenTagIDs lists tags whose events are excluded.
	HiddenTagIDs []uuid.UUID `json:"hidden_tag_ids"`
	// HideUntagged excludes events that have no tag.
	HideUntagged bool `json:"hide_untagged"`
}

// GetEventsFilter maps calendar names to event visibility filters.
type GetEventsFilter map[string]CalendarEventsFilter

func checkFilter(e *Event, filters GetEventsFilter) bool {
	filter, ok := filters[e.Calendar]
	if !ok {
		return true
	}

	if e.TagID == nil {
		return !filter.HideUntagged
	}

	return !slices.Contains(filter.HiddenTagIDs, *e.TagID)
}
