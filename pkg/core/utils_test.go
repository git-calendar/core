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

func Test_addUnit(t *testing.T) {
	type args struct {
		t     time.Time
		value int
		unit  Freq
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := addUnit(tt.args.t, tt.args.value, tt.args.unit); !cmp.Equal(tt.want, got) {
				t.Errorf("addUnit() = %v, want %v\ndiff=%s", got, tt.want, cmp.Diff(tt.want, got))
			}
		})
	}
}

func Test_firstOccurrenceAtOrAfter(t *testing.T) {
	type args struct {
		searchStart time.Time
		event       *Event
	}
	tests := []struct {
		name  string
		args  args
		want  time.Time
		want1 int
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := firstOccurrenceAtOrAfter(tt.args.searchStart, tt.args.event)
			if !cmp.Equal(tt.want, got) {
				t.Errorf("getFirstCandidate() got = %v, want %v\ndiff=%s", got, tt.want, cmp.Diff(tt.want, got))
			}
			if got1 != tt.want1 {
				t.Errorf("getFirstCandidate() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_containsTime(t *testing.T) {
	type args struct {
		exceptions []uuid.UUID
		t          time.Time
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsTime(tt.args.exceptions, tt.args.t); got != tt.want {
				t.Errorf("containsTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_prepareRepoUrl(t *testing.T) {
	someProxyUrl := mustParseUrl("https://cors-proxy.abc")
	tests := []struct {
		name     string
		repoUrl  url.URL
		proxyUrl *url.URL
		urlWant  url.URL
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
			proxyUrl: &someProxyUrl,
			urlWant:  mustParseUrl("https://cors-proxy.abc?url=https%3A%2F%2Fgithub.com%2Fjoe%2Fmy-calendar"),
			authWant: nil,
		},
		{
			name:     "basic proxy and token",
			repoUrl:  mustParseUrl("https://token_asdadad@github.com/joe/my-calendar"),
			proxyUrl: &someProxyUrl,
			urlWant:  mustParseUrl("https://cors-proxy.abc?url=https%3A%2F%2Fgithub.com%2Fjoe%2Fmy-calendar"),
			authWant: &http.BasicAuth{Username: "token_asdadad", Password: ""},
		},
		{
			name:     "basic proxy and username+pass",
			repoUrl:  mustParseUrl("https://joe:1234@github.com/joe/my-calendar"),
			proxyUrl: &someProxyUrl,
			urlWant:  mustParseUrl("https://cors-proxy.abc?url=https%3A%2F%2Fgithub.com%2Fjoe%2Fmy-calendar"),
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

func Test_useCorsProxy(t *testing.T) {
	tests := []struct {
		name     string
		original url.URL
		proxy    url.URL
		want     url.URL
	}{
		{
			name:     "basic proxy",
			original: mustParseUrl("https://github.com/joe/my-calendar"),
			proxy:    mustParseUrl("https://cors-proxy.abc"),
			want:     mustParseUrl("https://cors-proxy.abc?url=https%3A%2F%2Fgithub.com%2Fjoe%2Fmy-calendar"),
		},
		{
			name:     "basic proxy with trailing slash",
			original: mustParseUrl("https://github.com/joe/my-calendar"),
			proxy:    mustParseUrl("https://cors-proxy.abc/"),
			want:     mustParseUrl("https://cors-proxy.abc/?url=https%3A%2F%2Fgithub.com%2Fjoe%2Fmy-calendar"),
		},
		{
			name:     "proxy with another query param",
			original: mustParseUrl("https://github.com/joe/my-calendar"),
			proxy:    mustParseUrl("https://cors-proxy.abc?token=ABC123"),
			want:     mustParseUrl("https://cors-proxy.abc?token=ABC123&url=https%3A%2F%2Fgithub.com%2Fjoe%2Fmy-calendar"),
		},
		{
			name:     "proxy with another query param and trailing slash",
			original: mustParseUrl("https://github.com/joe/my-calendar"),
			proxy:    mustParseUrl("https://cors-proxy.abc/?token=ABC123"),
			want:     mustParseUrl("https://cors-proxy.abc/?token=ABC123&url=https%3A%2F%2Fgithub.com%2Fjoe%2Fmy-calendar"),
		},
		{
			name:     "query param in original url",
			original: mustParseUrl("https://github.com/joe/my-calendar?token=ABC123"),
			proxy:    mustParseUrl("https://cors-proxy.abc"),
			want:     mustParseUrl("https://cors-proxy.abc?url=https%3A%2F%2Fgithub.com%2Fjoe%2Fmy-calendar%3Ftoken%3DABC123"),
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

func Test_authFromUrl(t *testing.T) {
	tests := []struct {
		name string
		url  url.URL
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

func Test_calendarNameFromUrl(t *testing.T) {
	tests := []struct {
		name string
		url  url.URL
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

func Test_generateCustomUUID(t *testing.T) {
	type args struct {
		parentId uuid.UUID
		t        time.Time
	}
	tests := []struct {
		name string
		args args
		want uuid.UUID
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := generateCustomUUID(tt.args.parentId, tt.args.t); !cmp.Equal(tt.want, got) {
				t.Errorf("generateCustomUUID() = %v, want %v\ndiff=%s", got, tt.want, cmp.Diff(tt.want, got))
			}
		})
	}
}

func Test_getTimeFromUUID(t *testing.T) {
	type args struct {
		id uuid.UUID
	}
	tests := []struct {
		name    string
		args    args
		want    time.Time
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getTimeFromUUID(tt.args.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("getTimeFromUUID() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !cmp.Equal(tt.want, got) {
				t.Errorf("getTimeFromUUID() = %v, want %v\ndiff=%s", got, tt.want, cmp.Diff(tt.want, got))
			}
		})
	}
}

func Test_getShiftedUUID(t *testing.T) {
	type args struct {
		id       uuid.UUID
		duration time.Duration
	}
	tests := []struct {
		name string
		args args
		want uuid.UUID
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getShiftedUUID(tt.args.id, tt.args.duration); !cmp.Equal(tt.want, got) {
				t.Errorf("getShiftedUUID() = %v, want %v\ndiff=%s", got, tt.want, cmp.Diff(tt.want, got))
			}
		})
	}
}

// Helper
func mustParseUrl(raw string) url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(fmt.Sprintf("failed to parse url: %v", err))
	}
	return *u
}
