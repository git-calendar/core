package main

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"
)

func main() {
	prod := os.Getenv("PRODUCTION") == "true" // TODO better config
	var logger *slog.Logger
	if prod {
		// JSON logging
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: true}))
	} else {
		// basic logging
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	slog.SetDefault(logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/", proxyHandler)

	s := &http.Server{
		Addr:           ":8000",
		Handler:        accessLog(mux),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	slog.Info("Cors Proxy is running on " + s.Addr)
	log.Fatal(s.ListenAndServe())
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	// add cors headers to allow any browser to use this endpoint
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "*")

	if r.Method == http.MethodOptions {
		// this prevents the 405 from e.g. GitHub
		w.WriteHeader(http.StatusOK)
		return
	}

	// get the destination url from query
	destUrlQuery := r.URL.Query().Get("url")
	if destUrlQuery == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "'url' query param is missing\n")
		return
	}
	destUrl, err := url.ParseRequestURI(destUrlQuery)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "'url' query param is invalid\n")
		return
	}

	// prepare the request to destination
	req, err := http.NewRequestWithContext(r.Context(), r.Method, destUrl.String(), r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		slog.Error(err.Error())
		return
	}
	copyHeaders(r.Header, req.Header)
	removeHopByHopHeaders(req.Header)

	// send the actual request
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, "no such host\n")
		slog.Error(err.Error())
		return
	}

	// forward the headers and body back to client
	removeHopByHopHeaders(resp.Header)
	copyHeaders(resp.Header, w.Header())
	w.WriteHeader(resp.StatusCode)

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	err = resp.Body.Close()
	if err != nil {
		slog.Error(err.Error())
		return
	}
}
