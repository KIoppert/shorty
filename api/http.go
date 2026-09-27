package main

import (
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const codeAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

type App struct {
	cfg   Config
	store *Store
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", a.health)

	mux.HandleFunc("POST /api/auth/register", a.register)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("POST /api/auth/logout", a.logout)
	mux.HandleFunc("GET /api/me", a.requireUser(a.me))
	mux.HandleFunc("DELETE /api/me", a.requireUser(a.deleteMe))

	mux.HandleFunc("GET /api/links", a.requireUser(a.listLinks))
	mux.HandleFunc("POST /api/links", a.requireUser(a.createLink))
	mux.HandleFunc("GET /api/links/{id}", a.requireUser(a.getLink))
	mux.HandleFunc("PUT /api/links/{id}", a.requireUser(a.updateLink))
	mux.HandleFunc("DELETE /api/links/{id}", a.requireUser(a.deleteLink))
	mux.HandleFunc("GET /api/links/{id}/stats", a.requireUser(a.linkStats))
	mux.HandleFunc("GET /api/links/{id}/qr", a.requireUser(a.linkQR))

	mux.HandleFunc("GET /{code}", a.redirect)

	return logRequests(mux)
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	if err := a.store.db.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "ссылка не найдена")
		return 0, false
	}
	return id, true
}

func randomCode(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(b)
}

func referrerHost(r *http.Request) string {
	u, err := url.Parse(r.Referer())
	if err != nil {
		return ""
	}
	return u.Host
}

func device(ua string) string {
	ua = strings.ToLower(ua)
	switch {
	case strings.Contains(ua, "bot"), strings.Contains(ua, "spider"), strings.Contains(ua, "curl"):
		return "bot"
	case strings.Contains(ua, "ipad"), strings.Contains(ua, "tablet"):
		return "tablet"
	case strings.Contains(ua, "mobi"), strings.Contains(ua, "android"):
		return "mobile"
	default:
		return "desktop"
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func serverError(w http.ResponseWriter, err error) {
	slog.Error("internal error", "err", err)
	writeError(w, http.StatusInternalServerError, "что-то пошло не так, попробуйте ещё раз")
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
