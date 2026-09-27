package main

import (
	"errors"
	"net/http"
)

func (a *App) adminOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := a.store.Overview(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (a *App) adminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.AdminUsers(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

type accessInput struct {
	Banned  *bool `json:"banned"`
	IsAdmin *bool `json:"is_admin"`
}

func (a *App) adminUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := a.otherUserID(w, r)
	if !ok {
		return
	}
	var in accessInput
	if !readJSON(w, r, &in) {
		return
	}
	user, err := a.store.UpdateUserAccess(r.Context(), id, in.Banned, in.IsAdmin)
	if !a.adminResult(w, err, "пользователь не найден") {
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (a *App) adminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := a.otherUserID(w, r)
	if !ok {
		return
	}
	if a.adminResult(w, a.store.DeleteUser(r.Context(), id), "пользователь не найден") {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *App) adminLinks(w http.ResponseWriter, r *http.Request) {
	links, err := a.store.AdminLinks(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		serverError(w, err)
		return
	}
	for i := range links {
		a.fillShortURL(&links[i].Link)
	}
	writeJSON(w, http.StatusOK, links)
}

func (a *App) adminUpdateLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if a.adminResult(w, a.store.SetLinkEnabled(r.Context(), id, in.Enabled), "ссылка не найдена") {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *App) adminDeleteLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if a.adminResult(w, a.store.AdminDeleteLink(r.Context(), id), "ссылка не найдена") {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *App) otherUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := pathID(w, r)
	if ok && id == userID(r) {
		writeError(w, http.StatusBadRequest, "нельзя менять собственный аккаунт через админку")
		return 0, false
	}
	return id, ok
}

func (a *App) adminResult(w http.ResponseWriter, err error, notFound string) bool {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, notFound)
		return false
	case err != nil:
		serverError(w, err)
		return false
	}
	return true
}
