package main

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrDuplicate = errors.New("already exists")
)

type User struct {
	ID           int64      `json:"id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	IsAdmin      bool       `json:"is_admin"`
	BannedAt     *time.Time `json:"banned_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Link struct {
	ID        int64      `json:"id"`
	Code      string     `json:"code"`
	URL       string     `json:"url"`
	Title     string     `json:"title"`
	Enabled   bool       `json:"enabled"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
	Clicks    int64      `json:"clicks"`
	ShortURL  string     `json:"short_url"`
}

type Click struct {
	Referrer  string    `json:"referrer"`
	Device    string    `json:"device"`
	ClickedAt time.Time `json:"clicked_at"`
}

type DayCount struct {
	Day   time.Time `json:"day"`
	Count int64     `json:"count"`
}

type Stats struct {
	Total  int64      `json:"total"`
	Daily  []DayCount `json:"daily"`
	Recent []Click    `json:"recent"`
}

type Store struct {
	db *pgxpool.Pool
}

const userColumns = `u.id, u.email, u.password_hash, u.is_admin, u.banned_at, u.created_at`

func scanUser(row pgx.Row, extra ...any) (User, error) {
	var u User
	err := row.Scan(append([]any{&u.ID, &u.Email, &u.PasswordHash, &u.IsAdmin, &u.BannedAt, &u.CreatedAt}, extra...)...)
	return u, mapErr(err)
}

func (s *Store) CreateUser(ctx context.Context, email, hash string) (User, error) {
	return scanUser(s.db.QueryRow(ctx,
		`INSERT INTO users AS u (email, password_hash) VALUES ($1, $2) RETURNING `+userColumns,
		email, hash))
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(s.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users u WHERE u.email = $1`, email))
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	return scanUser(s.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users u WHERE u.id = $1`, id))
}

func (s *Store) PromoteUser(ctx context.Context, email string) error {
	return affected(s.db.Exec(ctx, `UPDATE users SET is_admin = TRUE WHERE email = $1`, email))
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	return affected(s.db.Exec(ctx, `DELETE FROM users WHERE id = $1`, id))
}

const linkColumns = `l.id, l.code, l.url, l.title, l.enabled, l.expires_at, l.created_at,
	(SELECT count(*) FROM clicks c WHERE c.link_id = l.id)`

func scanLink(row pgx.Row, extra ...any) (Link, error) {
	var l Link
	err := row.Scan(append([]any{&l.ID, &l.Code, &l.URL, &l.Title, &l.Enabled, &l.ExpiresAt, &l.CreatedAt, &l.Clicks}, extra...)...)
	return l, mapErr(err)
}

func (s *Store) CreateLink(ctx context.Context, userID int64, l Link) (Link, error) {
	err := s.db.QueryRow(ctx,
		`INSERT INTO links (user_id, code, url, title, expires_at) VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, enabled, created_at`,
		userID, l.Code, l.URL, l.Title, l.ExpiresAt,
	).Scan(&l.ID, &l.Enabled, &l.CreatedAt)
	return l, mapErr(err)
}

func (s *Store) ListLinks(ctx context.Context, userID int64) ([]Link, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+linkColumns+` FROM links l WHERE l.user_id = $1 ORDER BY l.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	links := []Link{}
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

func (s *Store) GetLink(ctx context.Context, userID, id int64) (Link, error) {
	return scanLink(s.db.QueryRow(ctx,
		`SELECT `+linkColumns+` FROM links l WHERE l.id = $1 AND l.user_id = $2`, id, userID))
}

func (s *Store) UpdateLink(ctx context.Context, userID int64, l Link) (Link, error) {
	return scanLink(s.db.QueryRow(ctx,
		`UPDATE links l SET url = $3, title = $4, enabled = $5, expires_at = $6
		 WHERE l.id = $1 AND l.user_id = $2
		 RETURNING `+linkColumns,
		l.ID, userID, l.URL, l.Title, l.Enabled, l.ExpiresAt))
}

func (s *Store) DeleteLink(ctx context.Context, userID, id int64) error {
	return affected(s.db.Exec(ctx, `DELETE FROM links WHERE id = $1 AND user_id = $2`, id, userID))
}

func (s *Store) LinkByCode(ctx context.Context, code string) (Link, error) {
	return scanLink(s.db.QueryRow(ctx,
		`SELECT `+linkColumns+` FROM links l JOIN users u ON u.id = l.user_id
		 WHERE l.code = $1 AND u.banned_at IS NULL`, code))
}

func (s *Store) RecordClick(ctx context.Context, linkID int64, referrer, device string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO clicks (link_id, referrer, device) VALUES ($1, $2, $3)`, linkID, referrer, device)
	return err
}

func (s *Store) LinkStats(ctx context.Context, linkID int64) (Stats, error) {
	stats := Stats{Daily: []DayCount{}, Recent: []Click{}}

	rows, err := s.db.Query(ctx,
		`SELECT d::date, count(c.id)
		 FROM generate_series(current_date - 13, current_date, interval '1 day') d
		 LEFT JOIN clicks c ON c.link_id = $1 AND c.clicked_at::date = d::date
		 GROUP BY d ORDER BY d`, linkID)
	if err != nil {
		return stats, err
	}
	for rows.Next() {
		var d DayCount
		if err := rows.Scan(&d.Day, &d.Count); err != nil {
			rows.Close()
			return stats, err
		}
		stats.Daily = append(stats.Daily, d)
	}
	rows.Close()

	rows, err = s.db.Query(ctx,
		`SELECT referrer, device, clicked_at FROM clicks WHERE link_id = $1 ORDER BY clicked_at DESC LIMIT 20`, linkID)
	if err != nil {
		return stats, err
	}
	defer rows.Close()
	for rows.Next() {
		var c Click
		if err := rows.Scan(&c.Referrer, &c.Device, &c.ClickedAt); err != nil {
			return stats, err
		}
		stats.Recent = append(stats.Recent, c)
	}

	err = s.db.QueryRow(ctx, `SELECT count(*) FROM clicks WHERE link_id = $1`, linkID).Scan(&stats.Total)
	return stats, err
}

type Overview struct {
	Users       int64 `json:"users"`
	Banned      int64 `json:"banned"`
	Links       int64 `json:"links"`
	Clicks      int64 `json:"clicks"`
	ClicksToday int64 `json:"clicks_today"`
}

type AdminUser struct {
	User
	Links  int64 `json:"links"`
	Clicks int64 `json:"clicks"`
}

type AdminLink struct {
	Link
	OwnerID     int64  `json:"owner_id"`
	OwnerEmail  string `json:"owner_email"`
	OwnerBanned bool   `json:"owner_banned"`
}

func (s *Store) Overview(ctx context.Context) (Overview, error) {
	var o Overview
	err := s.db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM users),
		(SELECT count(*) FROM users WHERE banned_at IS NOT NULL),
		(SELECT count(*) FROM links),
		(SELECT count(*) FROM clicks),
		(SELECT count(*) FROM clicks WHERE clicked_at >= current_date)`,
	).Scan(&o.Users, &o.Banned, &o.Links, &o.Clicks, &o.ClicksToday)
	return o, err
}

func (s *Store) AdminUsers(ctx context.Context, query string) ([]AdminUser, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+userColumns+`,
			(SELECT count(*) FROM links l WHERE l.user_id = u.id),
			(SELECT count(*) FROM clicks c JOIN links l ON l.id = c.link_id WHERE l.user_id = u.id)
		 FROM users u WHERE u.email ILIKE '%' || $1 || '%'
		 ORDER BY u.created_at DESC LIMIT 200`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []AdminUser{}
	for rows.Next() {
		var au AdminUser
		au.User, err = scanUser(rows, &au.Links, &au.Clicks)
		if err != nil {
			return nil, err
		}
		users = append(users, au)
	}
	return users, rows.Err()
}

func (s *Store) UpdateUserAccess(ctx context.Context, id int64, banned, isAdmin *bool) (User, error) {
	return scanUser(s.db.QueryRow(ctx,
		`UPDATE users u SET
			banned_at = CASE WHEN $2::bool IS NULL THEN banned_at WHEN $2 THEN coalesce(banned_at, now()) END,
			is_admin = coalesce($3, is_admin)
		 WHERE u.id = $1 RETURNING `+userColumns,
		id, banned, isAdmin))
}

func (s *Store) AdminLinks(ctx context.Context, query string) ([]AdminLink, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+linkColumns+`, u.id, u.email, u.banned_at IS NOT NULL FROM links l JOIN users u ON u.id = l.user_id
		 WHERE l.code ILIKE '%' || $1 || '%' OR l.url ILIKE '%' || $1 || '%' OR u.email ILIKE '%' || $1 || '%'
		 ORDER BY l.created_at DESC LIMIT 200`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	links := []AdminLink{}
	for rows.Next() {
		var al AdminLink
		al.Link, err = scanLink(rows, &al.OwnerID, &al.OwnerEmail, &al.OwnerBanned)
		if err != nil {
			return nil, err
		}
		links = append(links, al)
	}
	return links, rows.Err()
}

func (s *Store) SetLinkEnabled(ctx context.Context, id int64, enabled bool) error {
	return affected(s.db.Exec(ctx, `UPDATE links SET enabled = $2 WHERE id = $1`, id, enabled))
}

func (s *Store) AdminDeleteLink(ctx context.Context, id int64) error {
	return affected(s.db.Exec(ctx, `DELETE FROM links WHERE id = $1`, id))
}

func affected(tag pgconn.CommandTag, err error) error {
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicate
	}
	return err
}
