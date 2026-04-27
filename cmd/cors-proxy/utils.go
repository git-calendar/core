package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/sethvargo/go-envconfig"
	"github.com/sethvargo/go-limiter"
	"github.com/sethvargo/go-limiter/httplimit"
	"github.com/sethvargo/go-limiter/memorystore"
	"github.com/sethvargo/go-limiter/noopstore"
)

// a transport used for destination requests
var roundTripper http.RoundTripper = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConnsPerHost:   1000,
	IdleConnTimeout:       60 * time.Second,
	TLSHandshakeTimeout:   5 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ResponseHeaderTimeout: 10 * time.Second,
	ForceAttemptHTTP2:     true,
}

func isAllowedHost(u *url.URL) bool {
	if u == nil {
		return false
	}

	host := strings.ToLower(u.Hostname())
	return slices.Contains(cfg.AllowedHosts, host)
}

// ------------------- middleware -------------------

// Rate limits by IP.
func rateLimit(next http.Handler) http.Handler {
	var err error
	var store limiter.Store

	if cfg.Production {
		store, err = memorystore.New(&memorystore.Config{
			Tokens:   cfg.RateTokens,
			Interval: cfg.RateInterval,
		})
	} else {
		store, err = noopstore.New()
	}
	if err != nil {
		panic(err)
	}

	var limitFunc httplimit.KeyFunc
	if cfg.IpSourceHeader != "" {
		limitFunc = httplimit.IPKeyFunc(cfg.IpSourceHeader)
	} else {
		limitFunc = httplimit.IPKeyFunc()
	}

	middleware, err := httplimit.NewMiddleware(store, limitFunc)
	if err != nil {
		panic(err)
	}

	return middleware.Handle(next)
}

// Adds CORS headers to allow any browser to use this endpoint.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*") // TODO: add real https://git-calendar.firu.dev or whatever
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Git-Protocol")

		// handle preflight OPTIONS request
		if r.Method == http.MethodOptions {
			// this prevents the 405 from e.g., GitHub
			// the browser only needs to get the CORS headers and OK for OPTIONS request,
			// so that it knows it's safe to send the real request
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r) // next handler
	})
}

// Logs access after execution.
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now() // start timer

		ww := &responseWriter{ResponseWriter: w, status: http.StatusOK} // wrap the writer into our custom one
		next.ServeHTTP(ww, r)                                           // next handler

		duration := time.Since(start) // stop timer

		slog.Info("access",
			"method", r.Method,
			"status", ww.status,
			"duration", duration,
		)
	})
}

// A http.ResponseWriter wrapper, which catches the status code for logging.
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

type config struct {
	Host            string        `env:"HOST,default=0.0.0.0"`
	Port            string        `env:"PORT,default=8080"`
	Production      bool          `env:"PRODUCTION,default=false"`
	UpstreamTimeout time.Duration `env:"UPSTREAM_TIMEOUT,default=15s"`
	MaxResponseSize int64         `env:"MAX_RESPONSE_SIZE,default=1048576"` // 1MiB
	AllowedHosts    []string      `env:"ALLOWED_HOSTS,default=github.com,raw.githubusercontent.com,gitlab.com,codeberg.org"`
	RateTokens      uint64        `env:"RATE_TOKENS,default=40"` // 40 req/min should be ok for legit usage
	RateInterval    time.Duration `env:"RATE_INTERVAL,default=1m"`
	IpSourceHeader  string        `env:"RATE_IP_SOURCE_HEADER"` // for reverse proxy etc.
}

func loadConfig() {
	cfg = envconfig.MustProcess(context.Background(), &config{})

	// trim spaces for hosts
	for i := range cfg.AllowedHosts {
		cfg.AllowedHosts[i] = strings.TrimSpace(cfg.AllowedHosts[i])
	}
}
