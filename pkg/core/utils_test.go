package core

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

func TestPrepareRepoURL(t *testing.T) {
	someProxyURL := mustParseURL("https://cors-proxy.abc")
	tests := []struct {
		name     string
		repoURL  *url.URL
		proxyURL *url.URL
		urlWant  *url.URL
		authWant *http.BasicAuth
	}{
		{
			name:     "no proxy and no auth",
			repoURL:  mustParseURL("https://github.com/joe/my-calendar"),
			proxyURL: nil,
			urlWant:  mustParseURL("https://github.com/joe/my-calendar"),
			authWant: nil,
		},
		{
			name:     "basic proxy and no auth",
			repoURL:  mustParseURL("https://github.com/joe/my-calendar"),
			proxyURL: someProxyURL,
			urlWant:  mustParseURL("https://cors-proxy.abc/https://github.com/joe/my-calendar"),
			authWant: nil,
		},
		{
			name:     "basic proxy and token",
			repoURL:  mustParseURL("https://token_asdadad@github.com/joe/my-calendar"),
			proxyURL: someProxyURL,
			urlWant:  mustParseURL("https://cors-proxy.abc/https://github.com/joe/my-calendar"),
			authWant: &http.BasicAuth{Username: "token_asdadad", Password: ""},
		},
		{
			name:     "basic proxy and username+pass",
			repoURL:  mustParseURL("https://joe:1234@github.com/joe/my-calendar"),
			proxyURL: someProxyURL,
			urlWant:  mustParseURL("https://cors-proxy.abc/https://github.com/joe/my-calendar"),
			authWant: &http.BasicAuth{Username: "joe", Password: "1234"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			urlGot, authGot := prepareRepoURL(tt.repoURL, tt.proxyURL)
			if !cmp.Equal(tt.urlWant, urlGot) {
				t.Errorf("prepareRepoURL() got = %v, want %v\ndiff=%s", urlGot, tt.urlWant.String(), cmp.Diff(tt.urlWant, urlGot))
			}
			if !cmp.Equal(tt.authWant, authGot) {
				t.Errorf("prepareRepoURL() got1 = %v, want %v\ndiff=%s", authGot, tt.authWant, cmp.Diff(tt.authWant, authGot))
			}
		})
	}
}

func TestUseCorsProxy(t *testing.T) {
	tests := []struct {
		name     string
		original *url.URL
		proxy    *url.URL
		want     *url.URL
	}{
		{
			name:     "basic proxy",
			original: mustParseURL("https://github.com/joe/my-calendar.git"),
			proxy:    mustParseURL("http://cors-proxy.abc"),
			want:     mustParseURL("http://cors-proxy.abc/https://github.com/joe/my-calendar.git"),
		},
		{
			name:     "basic proxy with trailing slash",
			original: mustParseURL("https://github.com/joe/my-calendar.git"),
			proxy:    mustParseURL("http://cors-proxy.abc/"),
			want:     mustParseURL("http://cors-proxy.abc/https://github.com/joe/my-calendar.git"),
		},
		{
			name:     "query param in original url",
			original: mustParseURL("https://github.com/joe/my-calendar.git?token=ABC123"),
			proxy:    mustParseURL("http://cors-proxy.abc"),
			want:     mustParseURL("http://cors-proxy.abc/https://github.com/joe/my-calendar.git?token=ABC123"),
		},
		{
			name:     "proxy with path",
			original: mustParseURL("https://github.com/joe/my-calendar.git"),
			proxy:    mustParseURL("http://cors-proxy.abc/foo"),
			want:     mustParseURL("http://cors-proxy.abc/foo/https://github.com/joe/my-calendar.git"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := useCorsProxy(tt.original, tt.proxy); !cmp.Equal(tt.want, got) {
				t.Errorf("useCorsProxy() = %v, want %v\ndiff=%s", got.String(), tt.want.String(), cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestAuthFromURL(t *testing.T) {
	tests := []struct {
		name string
		url  *url.URL
		want *http.BasicAuth
	}{
		{
			name: "no auth",
			url:  mustParseURL("https://github.com/joe/my-calendar"),
			want: nil,
		},
		{
			name: "only token",
			url:  mustParseURL("https://token123@github.com/joe/my-calendar"),
			want: &http.BasicAuth{Username: "token123", Password: ""},
		},
		{
			name: "username and password",
			url:  mustParseURL("https://joe:password123@github.com/joe/my-calendar"),
			want: &http.BasicAuth{Username: "joe", Password: "password123"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authFromURL(tt.url); !cmp.Equal(tt.want, got) {
				t.Errorf("authFromURL() = %v, want %v\ndiff=%s", got, tt.want, cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestCalendarNameFromURL(t *testing.T) {
	tests := []struct {
		name string
		url  *url.URL
		want string
	}{
		{
			name: "basic",
			url:  mustParseURL("https://github.com/joe/my-calendar"),
			want: "my-calendar",
		},
		{
			name: "basic.git",
			url:  mustParseURL("https://github.com/joe/my-calendar.git"),
			want: "my-calendar",
		},
		{
			name: "trailing slash",
			url:  mustParseURL("https://github.com/joe/my-calendar/"),
			want: "my-calendar",
		},
		{
			name: "query params",
			url:  mustParseURL("https://github.com/joe/my-calendar?foo=1"),
			want: "my-calendar",
		},
		{
			name: "query params and trailing slash",
			url:  mustParseURL("https://github.com/joe/my-calendar/?foo=1"),
			want: "my-calendar",
		},
		{
			name: "empty",
			url:  mustParseURL(""),
			want: "shouldnthappen",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calendarNameFromURL(tt.url); got != tt.want {
				t.Errorf("calendarNameFromURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCustomUUIDs(t *testing.T) {
	tests := []struct {
		name     string
		parentID uuid.UUID
		t        time.Time
	}{
		{
			name:     "basic",
			parentID: uuid.New(), // UUIDv4
			t:        time.Now().Round(time.Second),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID := generateCustomUUID(tt.parentID, tt.t)
			gotTime := getTimeFromUUID(gotID)
			if !cmp.Equal(tt.t, gotTime) {
				t.Errorf("getTimeFromUUID() = %v, want %v", gotTime, tt.t)
			}
		})
	}
}

// Test helper
func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(fmt.Sprintf("failed to parse url: %v", err))
	}
	return u
}
