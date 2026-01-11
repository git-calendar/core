package main

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ------------------- middleware -------------------

func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		duration := time.Since(start)

		slog.Info("access",
			"method", r.Method,
			// "path", r.URL.Path,
			// "query", r.URL.RawQuery,
			// "remote", r.RemoteAddr,
			"status", ww.status,
			"duration", duration,
		)
	})
}

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
