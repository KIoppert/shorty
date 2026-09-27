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
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
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

func (s *Store) CreateUser(ctx context.Context, email, hash string) (User, error) {
	u := User{Email: email}
	err := s.db.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id, created_at`,
		email, hash,
	).Scan(&u.ID, &u.CreatedAt)
	return u, mapErr(err)
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.db.QueryRow(ctx,
		`SELECT id, email, password_hash, created_at FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	return u, mapErr(err)
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	var u User
	err := s.db.QueryRow(ctx,
		`SELECT id, email, password_hash, created_at FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	return u, mapErr(err)
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}

const linkColumns = `l.id, l.code, l.url, l.title, l.enabled, l.expires_at, l.created_at,
	(SELECT count(*) FROM clicks c WHERE c.link_id = l.id)`

func scanLink(row pgx.Row) (Link, error) {
	var l Link
	err := row.Scan(&l.ID, &l.Code, &l.URL, &l.Title, &l.Enabled, &l.ExpiresAt, &l.CreatedAt, &l.Clicks)
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
	tag, err := s.db.Exec(ctx, `DELETE FROM links WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) LinkByCode(ctx context.Context, code string) (Link, error) {
	return scanLink(s.db.QueryRow(ctx,
		`SELECT `+linkColumns+` FROM links l WHERE l.code = $1`, code))
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
