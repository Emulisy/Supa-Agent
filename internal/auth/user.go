// Package auth manages user accounts, passwords, and access tokens.
package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
)

const maxEmailBytes = 254

var (
	// ErrInvalidEmail indicates that an email is not a single, bare email address.
	ErrInvalidEmail = errors.New("user: invalid email address")
	// ErrEmailExists indicates that the users table's unique constraint was violated.
	ErrEmailExists = errors.New("user: email already exists")
	// ErrNotFound indicates that no user has the requested email.
	ErrNotFound = errors.New("user: not found")
)

// User is the public account record. Passwords and hashes are never returned.
type User struct {
	ID        uint64    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserByID loads public account data. Callers supply an authorized user ID.
func (s *Service) UserByID(ctx context.Context, userID uint64) (*User, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("auth: service is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("user: find by ID: %w", err)
	}
	if userID == 0 {
		return nil, ErrNotFound
	}
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, created_at, updated_at FROM users WHERE id = ?`,
		userID,
	).Scan(&u.ID, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("user: find by ID: %w", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("user: find by ID: %w", err)
	}
	return &u, nil
}

// createUser persists the normalized email and password hash prepared by Register.
// The database's unique email constraint protects concurrent registrations.
func (s *Service) createUser(ctx context.Context, email, passwordHash string) (uint64, error) {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO users (email, password_hash) VALUES (?, ?)`,
		email, passwordHash,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			// MySQL's duplicate-entry message contains the submitted email. Expose
			// only the domain error rather than including that message.
			return 0, fmt.Errorf("user: create: %w", ErrEmailExists)
		}
		return 0, fmt.Errorf("user: create: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("user: read inserted ID: %w", err)
	}
	if id == 0 {
		return 0, errors.New("user: database returned an invalid inserted ID")
	}
	// The MySQL driver represents unsigned BIGINT insert IDs as int64. Casting
	// back to uint64 also preserves IDs whose high bit is set.
	return uint64(id), nil
}

// findByEmail reads public account data for an already-normalized email.
func (s *Service) findByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, created_at, updated_at FROM users WHERE email = ?`,
		email,
	).Scan(&u.ID, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("user: find by email: %w", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("user: find by email: %w", err)
	}
	return &u, nil
}

// findCredentialsByEmail keeps password hashes separate from public User values.
func (s *Service) findCredentialsByEmail(ctx context.Context, email string) (uint64, string, error) {
	var userID uint64
	var passwordHash string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, password_hash FROM users WHERE email = ?`,
		email,
	).Scan(&userID, &passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", fmt.Errorf("user: find credentials by email: %w", ErrNotFound)
	}
	if err != nil {
		return 0, "", fmt.Errorf("user: find credentials by email: %w", err)
	}
	return userID, passwordHash, nil
}

func normalizeEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" || len(email) > maxEmailBytes || !utf8.ValidString(email) {
		return "", ErrInvalidEmail
	}
	email = strings.ToLower(email)
	if len(email) > maxEmailBytes {
		return "", ErrInvalidEmail
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Name != "" || address.Address != email {
		return "", ErrInvalidEmail
	}
	return email, nil
}
