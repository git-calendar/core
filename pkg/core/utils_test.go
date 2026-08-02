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

func TestPrepareRepoUrl(t *testing.T) {
	someProxyUrl := mustParseUrl("https://cors-proxy.abc")
	tests := []struct {
		name     string
		repoUrl  *url.URL
		proxyUrl *url.URL
		urlWant  *url.URL
		authWant *http.BasicAuth
	}{
		{
			name:     "no proxy and no auth",
			repoUrl:  mustParseUrl("https://github.com/joe/my-calendar"),
			proxyUrl: nil,
			urlWant:  mustParseUrl("https://github.com/joe/my-calendar"),
			authWant: nil,
		},
		{
			name:     "basic proxy and no auth",
			repoUrl:  mustParseUrl("https://github.com/joe/my-calendar"),
			proxyUrl: someProxyUrl,
			urlWant:  mustParseUrl("https://cors-proxy.abc/https://github.com/joe/my-calendar"),
			authWant: nil,
		},
		{
			name:     "basic proxy and token",
			repoUrl:  mustParseUrl("https://token_asdadad@github.com/joe/my-calendar"),
			proxyUrl: someProxyUrl,
			urlWant:  mustParseUrl("https://cors-proxy.abc/https://github.com/joe/my-calendar"),
			authWant: &http.BasicAuth{Username: "token_asdadad", Password: ""},
		},
		{
			name:     "basic proxy and username+pass",
			repoUrl:  mustParseUrl("https://joe:1234@github.com/joe/my-calendar"),
			proxyUrl: someProxyUrl,
			urlWant:  mustParseUrl("https://cors-proxy.abc/https://github.com/joe/my-calendar"),
			authWant: &http.BasicAuth{Username: "joe", Password: "1234"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			urlGot, authGot := prepareRepoUrl(tt.repoUrl, tt.proxyUrl)
			if !cmp.Equal(tt.urlWant, urlGot) {
				t.Errorf("prepareRepoUrl() got = %v, want %v\ndiff=%s", urlGot, tt.urlWant.String(), cmp.Diff(tt.urlWant, urlGot))
			}
			if !cmp.Equal(tt.authWant, authGot) {
				t.Errorf("prepareRepoUrl() got1 = %v, want %v\ndiff=%s", authGot, tt.authWant, cmp.Diff(tt.authWant, authGot))
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
			original: mustParseUrl("https://github.com/joe/my-calendar.git"),
			proxy:    mustParseUrl("http://cors-proxy.abc"),
			want:     mustParseUrl("http://cors-proxy.abc/https://github.com/joe/my-calendar.git"),
		},
		{
			name:     "basic proxy with trailing slash",
			original: mustParseUrl("https://github.com/joe/my-calendar.git"),
			proxy:    mustParseUrl("http://cors-proxy.abc/"),
			want:     mustParseUrl("http://cors-proxy.abc/https://github.com/joe/my-calendar.git"),
		},
		{
			name:     "query param in original url",
			original: mustParseUrl("https://github.com/joe/my-calendar.git?token=ABC123"),
			proxy:    mustParseUrl("http://cors-proxy.abc"),
			want:     mustParseUrl("http://cors-proxy.abc/https://github.com/joe/my-calendar.git?token=ABC123"),
		},
		{
			name:     "proxy with path",
			original: mustParseUrl("https://github.com/joe/my-calendar.git"),
			proxy:    mustParseUrl("http://cors-proxy.abc/foo"),
			want:     mustParseUrl("http://cors-proxy.abc/foo/https://github.com/joe/my-calendar.git"),
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

func TestAuthFromUrl(t *testing.T) {
	tests := []struct {
		name string
		url  *url.URL
		want *http.BasicAuth
	}{
		{
			name: "no auth",
			url:  mustParseUrl("https://github.com/joe/my-calendar"),
			want: nil,
		},
		{
			name: "only token",
			url:  mustParseUrl("https://token123@github.com/joe/my-calendar"),
			want: &http.BasicAuth{Username: "token123", Password: ""},
		},
		{
			name: "username and password",
			url:  mustParseUrl("https://joe:password123@github.com/joe/my-calendar"),
			want: &http.BasicAuth{Username: "joe", Password: "password123"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authFromUrl(tt.url); !cmp.Equal(tt.want, got) {
				t.Errorf("authFromUrl() = %v, want %v\ndiff=%s", got, tt.want, cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestCalendarNameFromUrl(t *testing.T) {
	tests := []struct {
		name string
		url  *url.URL
		want string
	}{
		{
			name: "basic",
			url:  mustParseUrl("https://github.com/joe/my-calendar"),
			want: "my-calendar",
		},
		{
			name: "basic.git",
			url:  mustParseUrl("https://github.com/joe/my-calendar.git"),
			want: "my-calendar",
		},
		{
			name: "trailing slash",
			url:  mustParseUrl("https://github.com/joe/my-calendar/"),
			want: "my-calendar",
		},
		{
			name: "query params",
			url:  mustParseUrl("https://github.com/joe/my-calendar?foo=1"),
			want: "my-calendar",
		},
		{
			name: "query params and trailing slash",
			url:  mustParseUrl("https://github.com/joe/my-calendar/?foo=1"),
			want: "my-calendar",
		},
		{
			name: "empty",
			url:  mustParseUrl(""),
			want: "shouldnthappen",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calendarNameFromUrl(tt.url); got != tt.want {
				t.Errorf("calendarNameFromUrl() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCustomUUIDs(t *testing.T) {
	tests := []struct {
		name     string
		parentId uuid.UUID
		t        time.Time
	}{
		{
			name:     "basic",
			parentId: uuid.New(), // UUIDv4
			t:        time.Now().Round(time.Second),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotId := generateCustomUUID(tt.parentId, tt.t)
			gotTime := getTimeFromUUID(gotId)
			if !cmp.Equal(tt.t, gotTime) {
				t.Errorf("getTimeFromUUID() = %v, want %v", gotTime, tt.t)
			}
		})
	}
}

// Test helper
func mustParseUrl(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(fmt.Sprintf("failed to parse url: %v", err))
	}
	return u
}
