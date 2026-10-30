package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const probeAlias = "prober-check"

var (
	probeRuns = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "shorty_probe_total",
		Help: "Prober checks by check name and result: success, failure.",
	}, []string{"check", "result"})
	probeSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shorty_probe_success",
		Help: "Result of the last prober check: 1 success, 0 failure.",
	}, []string{"check"})
	probeDuration = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shorty_probe_duration_seconds",
		Help: "Duration of the last prober check.",
	}, []string{"check"})
)

type prober struct {
	target string
	creds  credentials
	client *http.Client
	ready  bool
}

func probe(ctx context.Context) error {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: logKeys})
	slog.SetDefault(slog.New(handler).With("service.name", "shorty-prober"))

	interval, err := time.ParseDuration(env("PROBE_INTERVAL", "10s"))
	if err != nil {
		return fmt.Errorf("PROBE_INTERVAL: %w", err)
	}
	jar, _ := cookiejar.New(nil)
	p := &prober{
		target: strings.TrimRight(env("PROBE_TARGET", "http://localhost:8080"), "/"),
		creds:  credentials{Email: env("PROBE_EMAIL", "prober@shorty.local"), Password: env("PROBE_PASSWORD", "prober-password")},
		client: &http.Client{
			Jar:           jar,
			Timeout:       2 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	checks := map[string]func(context.Context) error{
		"web":      p.web,
		"health":   p.health,
		"redirect": p.redirect,
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(probeRuns, probeSuccess, probeDuration)
	for name := range checks {
		probeRuns.WithLabelValues(name, "success")
		probeRuns.WithLabelValues(name, "failure")
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	srv := &http.Server{Addr: ":" + env("METRICS_PORT", "9091"), Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	errs := make(chan error, 1)
	go func() {
		slog.Info("probing", "target", p.target, "interval", interval.String(), "metrics", srv.Addr)
		errs <- srv.ListenAndServe()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		for name, check := range checks {
			p.run(ctx, name, check)
		}
		select {
		case err := <-errs:
			return err
		case <-ctx.Done():
			return srv.Shutdown(context.Background())
		case <-ticker.C:
		}
	}
}

func (p *prober) run(ctx context.Context, name string, check func(context.Context) error) {
	start := time.Now()
	err := check(ctx)
	probeDuration.WithLabelValues(name).Set(time.Since(start).Seconds())
	if err != nil {
		slog.Warn("probe failed", "check", name, "err", err)
		probeRuns.WithLabelValues(name, "failure").Inc()
		probeSuccess.WithLabelValues(name).Set(0)
		return
	}
	probeRuns.WithLabelValues(name, "success").Inc()
	probeSuccess.WithLabelValues(name).Set(1)
}

func (p *prober) web(ctx context.Context) error {
	return p.expect(ctx, http.MethodGet, "/", nil, http.StatusOK)
}

func (p *prober) health(ctx context.Context) error {
	return p.expect(ctx, http.MethodGet, "/healthz", nil, http.StatusOK)
}

func (p *prober) redirect(ctx context.Context) error {
	if !p.ready {
		if err := p.setup(ctx); err != nil {
			return fmt.Errorf("setup: %w", err)
		}
		p.ready = true
	}
	err := p.expect(ctx, http.MethodGet, "/"+probeAlias, nil, http.StatusFound)
	p.ready = err == nil
	return err
}

func (p *prober) setup(ctx context.Context) error {
	if err := p.expect(ctx, http.MethodPost, "/api/auth/register", p.creds, http.StatusOK, http.StatusConflict); err != nil {
		return err
	}
	if err := p.expect(ctx, http.MethodPost, "/api/auth/login", p.creds, http.StatusOK); err != nil {
		return err
	}
	link := linkInput{URL: "https://example.com/prober", Alias: probeAlias}
	return p.expect(ctx, http.MethodPost, "/api/links", link, http.StatusCreated, http.StatusConflict)
}

func (p *prober) expect(ctx context.Context, method, path string, body any, want ...int) error {
	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.target+path, payload)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if !slices.Contains(want, resp.StatusCode) {
		return fmt.Errorf("%s %s: status %d", method, path, resp.StatusCode)
	}
	return nil
}
