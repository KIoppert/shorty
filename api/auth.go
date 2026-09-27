package main

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const cookieName = "shorty_token"

type ctxKey struct{}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (a *App) setSession(w http.ResponseWriter, userID int64) error {
	expires := time.Now().Add(a.cfg.TokenTTL)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		ExpiresAt: jwt.NewNumericDate(expires),
	}).SignedString(a.cfg.JWTSecret)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (a *App) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *App) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "нужно войти")
			return
		}

		var claims jwt.RegisteredClaims
		_, err = jwt.ParseWithClaims(cookie.Value, &claims, func(*jwt.Token) (any, error) {
			return a.cfg.JWTSecret, nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "сессия истекла, войдите снова")
			return
		}

		id, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "сессия недействительна")
			return
		}

		user, err := a.store.UserByID(r.Context(), id)
		if errors.Is(err, ErrNotFound) {
			a.clearSession(w)
			writeError(w, http.StatusUnauthorized, "аккаунт не найден")
			return
		}
		if err != nil {
			serverError(w, err)
			return
		}
		if user.BannedAt != nil {
			a.clearSession(w)
			writeError(w, http.StatusUnauthorized, "аккаунт заблокирован")
			return
		}

		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	}
}

func (a *App) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return a.requireUser(func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).IsAdmin {
			writeError(w, http.StatusForbidden, "нужны права администратора")
			return
		}
		next(w, r)
	})
}

func currentUser(r *http.Request) User {
	return r.Context().Value(ctxKey{}).(User)
}

func userID(r *http.Request) int64 {
	return currentUser(r).ID
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !readJSON(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if _, err := mail.ParseAddress(in.Email); err != nil {
		writeError(w, http.StatusBadRequest, "введите корректный email")
		return
	}
	if len(in.Password) < 8 {
		writeError(w, http.StatusBadRequest, "пароль должен быть не короче 8 символов")
		return
	}

	hash, err := hashPassword(in.Password)
	if err != nil {
		serverError(w, err)
		return
	}
	user, err := a.store.CreateUser(r.Context(), in.Email, hash)
	if errors.Is(err, ErrDuplicate) {
		writeError(w, http.StatusConflict, "этот email уже зарегистрирован")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}

	a.startSession(w, user)
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !readJSON(w, r, &in) {
		return
	}
	user, err := a.store.UserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(in.Email)))
	if err != nil && !errors.Is(err, ErrNotFound) {
		serverError(w, err)
		return
	}
	if err != nil || !checkPassword(user.PasswordHash, in.Password) {
		writeError(w, http.StatusUnauthorized, "неверный email или пароль")
		return
	}
	if user.BannedAt != nil {
		writeError(w, http.StatusForbidden, "аккаунт заблокирован")
		return
	}

	a.startSession(w, user)
}

func (a *App) startSession(w http.ResponseWriter, user User) {
	if err := a.setSession(w, user.ID); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	a.clearSession(w)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r))
}

func (a *App) deleteMe(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteUser(r.Context(), userID(r)); err != nil {
		serverError(w, err)
		return
	}
	a.clearSession(w)
	w.WriteHeader(http.StatusNoContent)
}
