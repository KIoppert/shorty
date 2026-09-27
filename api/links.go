package main

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"
)

var (
	aliasPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
	reserved     = map[string]bool{"api": true, "healthz": true, "assets": true}
)

type linkInput struct {
	URL       string     `json:"url"`
	Title     string     `json:"title"`
	Alias     string     `json:"alias"`
	Enabled   bool       `json:"enabled"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func (in *linkInput) validate() string {
	in.URL = strings.TrimSpace(in.URL)
	in.Title = strings.TrimSpace(in.Title)
	in.Alias = strings.TrimSpace(in.Alias)

	u, err := url.ParseRequestURI(in.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "ссылка должна начинаться с http:// или https://"
	}
	if len(in.Title) > 200 {
		return "название не длиннее 200 символов"
	}
	if in.Alias != "" && (!aliasPattern.MatchString(in.Alias) || reserved[strings.ToLower(in.Alias)]) {
		return "алиас: 3–32 символа, латиница, цифры, - и _"
	}
	return ""
}

func (a *App) listLinks(w http.ResponseWriter, r *http.Request) {
	links, err := a.store.ListLinks(r.Context(), userID(r))
	if err != nil {
		serverError(w, err)
		return
	}
	for i := range links {
		a.fillShortURL(&links[i])
	}
	writeJSON(w, http.StatusOK, links)
}

func (a *App) createLink(w http.ResponseWriter, r *http.Request) {
	var in linkInput
	if !readJSON(w, r, &in) {
		return
	}
	if msg := in.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	link := Link{URL: in.URL, Title: in.Title, ExpiresAt: in.ExpiresAt}
	var err error
	for range 5 {
		link.Code = in.Alias
		if link.Code == "" {
			link.Code = randomCode(6)
		}
		link, err = a.store.CreateLink(r.Context(), userID(r), link)
		if !errors.Is(err, ErrDuplicate) || in.Alias != "" {
			break
		}
	}
	if errors.Is(err, ErrDuplicate) {
		writeError(w, http.StatusConflict, "такой алиас уже занят")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}

	a.fillShortURL(&link)
	writeJSON(w, http.StatusCreated, link)
}

func (a *App) getLink(w http.ResponseWriter, r *http.Request) {
	link, ok := a.findLink(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func (a *App) updateLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in linkInput
	if !readJSON(w, r, &in) {
		return
	}
	in.Alias = ""
	if msg := in.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	link, err := a.store.UpdateLink(r.Context(), userID(r), Link{
		ID: id, URL: in.URL, Title: in.Title, Enabled: in.Enabled, ExpiresAt: in.ExpiresAt,
	})
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "ссылка не найдена")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}

	a.fillShortURL(&link)
	writeJSON(w, http.StatusOK, link)
}

func (a *App) deleteLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := a.store.DeleteLink(r.Context(), userID(r), id)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "ссылка не найдена")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) linkStats(w http.ResponseWriter, r *http.Request) {
	link, ok := a.findLink(w, r)
	if !ok {
		return
	}
	stats, err := a.store.LinkStats(r.Context(), link.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *App) linkQR(w http.ResponseWriter, r *http.Request) {
	link, ok := a.findLink(w, r)
	if !ok {
		return
	}
	png, err := qrcode.Encode(link.ShortURL, qrcode.Medium, 512)
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write(png)
}

func (a *App) redirect(w http.ResponseWriter, r *http.Request) {
	link, err := a.store.LinkByCode(r.Context(), r.PathValue("code"))
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "Такой короткой ссылки нет", http.StatusNotFound)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if !link.Enabled || (link.ExpiresAt != nil && link.ExpiresAt.Before(time.Now())) {
		http.Error(w, "Ссылка больше не работает", http.StatusGone)
		return
	}

	if err := a.store.RecordClick(r.Context(), link.ID, referrerHost(r), device(r.UserAgent())); err != nil {
		slog.Error("record click", "err", err, "code", link.Code)
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (a *App) findLink(w http.ResponseWriter, r *http.Request) (Link, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return Link{}, false
	}
	link, err := a.store.GetLink(r.Context(), userID(r), id)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "ссылка не найдена")
		return link, false
	}
	if err != nil {
		serverError(w, err)
		return link, false
	}
	a.fillShortURL(&link)
	return link, true
}

func (a *App) fillShortURL(l *Link) {
	l.ShortURL = strings.TrimRight(a.cfg.BaseURL, "/") + "/" + l.Code
}
