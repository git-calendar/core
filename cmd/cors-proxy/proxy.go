package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

var cfg *config

func main() {
	// load config
	loadConfig()

	// setup logger
	var logger *slog.Logger
	if cfg.Production {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: true})) // JSON logging
	} else {
		logger = slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug, AddSource: true, TimeFormat: time.DateTime})) // basic logging
	}
	slog.SetDefault(logger)

	// create a http server
	mux := http.NewServeMux()
	mux.HandleFunc("/", proxyHandler)

	s := &http.Server{
		Addr:           fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Handler:        accessLog(corsMiddleware(rateLimit(mux))),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   15 * time.Second,
		MaxHeaderBytes: 64 << 10, // 1KB
	}

	slog.Info(fmt.Sprintf("configuration: %+v", *cfg))
	slog.Info("running on " + s.Addr)

	// run the proxy
	if err := s.ListenAndServe(); err != nil {
		panic(err)
	}
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	// get the destination url from query
	destUrlQuery := r.URL.Query().Get("url")
	if destUrlQuery == "" {
		http.Error(w, "'url' query param is missing", http.StatusBadRequest)
		return
	}
	destUrl, err := url.ParseRequestURI(destUrlQuery)
	if err != nil {
		http.Error(w, "'url' query param is invalid", http.StatusBadRequest)
		return
	}

	// reject unknown hosts
	if !isAllowedHost(destUrl) {
		http.Error(w, "forbidden upstream host", http.StatusForbidden)
		return
	}

	// prepare the request to destination
	req, err := http.NewRequest(r.Method, destUrl.String(), r.Body)
	if err != nil {
		http.Error(w, "failed to create outbound request", http.StatusInternalServerError)
		slog.Error(err.Error())
		return
	}
	copyHeaders(r.Header, req.Header)
	removeHopByHopHeaders(req.Header)

	// add context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), cfg.UpstreamTimeout)
	defer cancel()
	req = req.WithContext(ctx)

	// send the actual request
	resp, err := roundTripper.RoundTrip(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream request failed with: %v", err), http.StatusBadGateway)
		slog.Error(err.Error())
		return
	}
	defer resp.Body.Close()

	// forward the headers back to client
	removeHopByHopHeaders(resp.Header)
	copyHeaders(resp.Header, w.Header())
	w.WriteHeader(resp.StatusCode)

	// limit response body
	limitedReader := &io.LimitedReader{
		R: resp.Body,
		N: cfg.MaxResponseSize + 1, // to detect overflow
	}

	// forward response body back to client
	n, err := io.Copy(w, limitedReader)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	if n > cfg.MaxResponseSize {
		slog.Info("response too large")
		return
	}
}
