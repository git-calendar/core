package main

import (
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// a transport used for destination requests
var roundTripper http.RoundTripper = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:          100,
	IdleConnTimeout:       60 * time.Second,
	TLSHandshakeTimeout:   5 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ResponseHeaderTimeout: 10 * time.Second,
}

// TODO separate into a text file
var allowedHosts = map[string]bool{
	"github.com":                true,
	"raw.githubusercontent.com": true,
	"gitlab.com":                true,
	"codeberg.org":              true,
}

func isAllowedHost(u *url.URL) bool {
	if u == nil {
		return false
	}

	host := strings.ToLower(u.Hostname())
	_, ok := allowedHosts[host]
	return ok
}

// ------------------- middleware -------------------

func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now() // start timer

		ww := &responseWriter{ResponseWriter: w, status: http.StatusOK} // wrap the writer into our custom one
		next.ServeHTTP(ww, r)                                           // execute handler

		duration := time.Since(start) // stop timer

		slog.Info("access",
			"method", r.Method,
			"status", ww.status,
			"duration", duration,
		)
	})
}

// a http.ResponseWriter wrapper, which catches the status code for logging
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// ------------------- headers -------------------

// https://www.rfc-editor.org/rfc/rfc2616?ref=journal.hexmos.com#section-13.5.1
var hopByHopHeaders map[string]bool = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"TE":                  true,
	"Trailers":            true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

func copyHeaders(src, dst http.Header) {
	for name, vals := range src {
		for _, val := range vals {
			dst.Add(name, val)
		}
	}
}

func removeHopByHopHeaders(h http.Header) {
	// remove all headers listed in Connection header (https://www.rfc-editor.org/rfc/rfc2616?ref=journal.hexmos.com#section-14.10)
	connectionHeader := h.Get("Connection")
	if connectionHeader != "" {
		headersToRemove := strings.SplitSeq(connectionHeader, ",")
		for header := range headersToRemove {
			header = strings.TrimSpace(header)
			h.Del(header)
		}
	}
	// remove all hop-by-hop
	for name := range hopByHopHeaders {
		h.Del(name)
	}
}

// ------------------- config -------------------

// External library? https://github.com/caarlos0/env
// Overkill! (for now)

const prefix = "CORS_PROXY_"

type config struct {
	Host            string
	Port            string
	Production      bool
	UpstreamTimeout time.Duration
	MaxResponseSize int64
}

func loadConfig() *config {
	var cfg config

	prodEnv := os.Getenv(prefix + "PRODUCTION")
	cfg.Production = prodEnv == "true" || prodEnv == "1" || prodEnv == "True" || prodEnv == "TRUE"

	cfg.Host = os.Getenv(prefix + "HOST") // empty is the same as 0.0.0.0
	cfg.Port = os.Getenv(prefix + "PORT")
	if cfg.Port == "" {
		cfg.Port = "8000"
	}

	duration, err := time.ParseDuration(os.Getenv(prefix + "UPSTREAM_TIMEOUT"))
	if err != nil {
		cfg.UpstreamTimeout = 15 * time.Second
	} else {
		cfg.UpstreamTimeout = duration
	}

	cfg.MaxResponseSize, err = strconv.ParseInt(os.Getenv(prefix+"MAX_RESPONSE_SIZE"), 10, 64)
	if err != nil || cfg.MaxResponseSize == 0 {
		cfg.MaxResponseSize = 1 << 20 // 1MB
	}

	return &cfg
}
