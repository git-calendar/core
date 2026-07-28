package core

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/uuid"
)

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

func repoUrlFromCalendar(cal *Calendar) (*url.URL, error) {
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

type CalendarEventsFilter struct {
	HiddenTagIds []uuid.UUID `json:"hidden_tag_ids"`
	HideUntagged bool        `json:"hide_untagged"`
}

type GetEventsFilter map[string]CalendarEventsFilter

func checkFilter(e *Event, filters GetEventsFilter) bool {
	filter, ok := filters[e.Calendar]
	if !ok {
		return true
	}

	if e.TagId == nil {
		return !filter.HideUntagged
	}

	return !slices.Contains(filter.HiddenTagIds, *e.TagId)
}
