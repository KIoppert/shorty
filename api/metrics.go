package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shorty_http_requests_total",
		Help: "HTTP requests by route, method and status code.",
	}, []string{"route", "method", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "shorty_http_request_duration_seconds",
		Help:    "HTTP request duration by route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"route", "method"})

	usersRegistered = promauto.NewCounter(prometheus.CounterOpts{
		Name: "shorty_users_registered_total",
		Help: "Successful user registrations.",
	})

	logins = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shorty_logins_total",
		Help: "Login attempts by result: success, failure, banned.",
	}, []string{"result"})

	linksCreated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shorty_links_created_total",
		Help: "Created short links by code type: custom alias or random.",
	}, []string{"type"})

	redirects = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shorty_redirects_total",
		Help: "Short link visits by result: ok, not_found, gone.",
	}, []string{"result"})

	qrCodes = promauto.NewCounter(prometheus.CounterOpts{
		Name: "shorty_qr_codes_generated_total",
		Help: "Generated QR codes.",
	})
)

func init() {
	for _, result := range []string{"success", "failure", "banned"} {
		logins.WithLabelValues(result)
	}
	for _, kind := range []string{"custom", "random"} {
		linksCreated.WithLabelValues(kind)
	}
	for _, result := range []string{"ok", "not_found", "gone"} {
		redirects.WithLabelValues(result)
	}
}

func metricsHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	return mux
}

func observeRequest(r *http.Request, status int, took time.Duration) {
	route := r.Pattern
	if route == "" {
		route = "unmatched"
	}
	httpRequests.WithLabelValues(route, r.Method, strconv.Itoa(status)).Inc()
	httpDuration.WithLabelValues(route, r.Method).Observe(took.Seconds())
}
