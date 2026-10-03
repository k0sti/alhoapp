package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrDuplicatePubkey = errors.New("this key is already registered to a member")
	ErrDuplicateEmail  = errors.New("this email address is already registered")
	ErrNotFound        = errors.New("member not found")
)

// Member is one community member. Ncryptsec is the NIP-49 encrypted secret key
// for members whose key is stored on the server; it is empty for self-hosted keys.
// The server cannot decrypt it: the password never leaves the browser.
type Member struct {
	Pubkey     string     `json:"pubkey"`
	Name       string     `json:"name"`
	Email      string     `json:"email"`
	Ncryptsec  string     `json:"-"`
	Verified   bool       `json:"verified"`
	CreatedAt  time.Time  `json:"createdAt"`
	VerifiedAt *time.Time `json:"verifiedAt,omitempty"`
}

func (m *Member) KeyStorage() string {
	if m.Ncryptsec != "" {
		return "server"
	}
	return "self"
}

type MemberStore struct {
	db *sql.DB
}

func OpenMemberStore(path string) (*MemberStore, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS members (
			pubkey       TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			email        TEXT NOT NULL UNIQUE COLLATE NOCASE,
			ncryptsec    TEXT,
			verified     INTEGER NOT NULL DEFAULT 0,
			verify_token TEXT UNIQUE,
			created_at   INTEGER NOT NULL,
			verified_at  INTEGER
		)`)
	if err != nil {
		return nil, err
	}
	return &MemberStore{db: db}, nil
}

func (s *MemberStore) Close() error { return s.db.Close() }

// Create inserts a new member and returns the email confirmation token.
func (s *MemberStore) Create(ctx context.Context, m *Member) (token string, err error) {
	token = randomToken()
	m.CreatedAt = time.Now()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO members (pubkey, name, email, ncryptsec, verify_token, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		m.Pubkey, m.Name, m.Email, nullable(m.Ncryptsec), token, m.CreatedAt.Unix())
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "members.pubkey"):
			return "", ErrDuplicatePubkey
		case strings.Contains(msg, "members.email"):
			return "", ErrDuplicateEmail
		}
		return "", err
	}
	return token, nil
}

const memberColumns = `pubkey, name, email, ncryptsec, verified, created_at, verified_at`

func scanMember(row interface{ Scan(...any) error }) (*Member, error) {
	var m Member
	var ncryptsec sql.NullString
	var createdAt int64
	var verifiedAt sql.NullInt64
	if err := row.Scan(&m.Pubkey, &m.Name, &m.Email, &ncryptsec, &m.Verified, &createdAt, &verifiedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	m.Ncryptsec = ncryptsec.String
	m.CreatedAt = time.Unix(createdAt, 0)
	if verifiedAt.Valid {
		t := time.Unix(verifiedAt.Int64, 0)
		m.VerifiedAt = &t
	}
	return &m, nil
}

func (s *MemberStore) ByPubkey(ctx context.Context, pubkey string) (*Member, error) {
	return scanMember(s.db.QueryRowContext(ctx, `SELECT `+memberColumns+` FROM members WHERE pubkey = ?`, pubkey))
}

func (s *MemberStore) ByEmail(ctx context.Context, email string) (*Member, error) {
	return scanMember(s.db.QueryRowContext(ctx, `SELECT `+memberColumns+` FROM members WHERE email = ?`, email))
}

func (s *MemberStore) Exists(ctx context.Context, pubkey string) bool {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM members WHERE pubkey = ?`, pubkey).Scan(&one)
	return err == nil
}

func (s *MemberStore) List(ctx context.Context) ([]*Member, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+memberColumns+` FROM members ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Member
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Verify marks the member owning token as verified. The token stays valid so
// that following the link twice is harmless.
func (s *MemberStore) Verify(ctx context.Context, token string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE members SET verified = 1, verified_at = COALESCE(verified_at, ?) WHERE verify_token = ?`,
		time.Now().Unix(), token)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MemberStore) VerifyToken(ctx context.Context, pubkey string) (string, error) {
	var token string
	err := s.db.QueryRowContext(ctx, `SELECT verify_token FROM members WHERE pubkey = ?`, pubkey).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return token, err
}

func (s *MemberStore) Delete(ctx context.Context, pubkey string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM members WHERE pubkey = ?`, pubkey)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func randomToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
