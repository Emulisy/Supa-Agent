// Package user provides MySQL-backed user creation and lookup.
package user

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
	"golang.org/x/crypto/bcrypt"
)

const (
	MinPasswordLength = 8

	MaxPasswordBytes = 72

	maxEmailBytes = 254
)

var (
	// ErrInvalidEmail indicates that an email is not a single, bare email address.
	ErrInvalidEmail = errors.New("user: invalid email address")
	// ErrInvalidPassword indicates that a password does not meet the input policy.
	ErrInvalidPassword = errors.New("user: invalid password")
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

// dependency injection
type Repository struct {
	db *sql.DB
}

// NewRepository constructs a repository without opening or closing connections.
// Pass infra.Client.DB when using the project's MySQL client.
func NewRepository(db *sql.DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("user: database is required")
	}
	return &Repository{db: db}, nil
}

// Create validates the inputs, hashes the plaintext password with bcrypt, and
// returns the generated ID. Email addresses are trimmed and treated as
// case-insensitive account identifiers. Passwords are used exactly as supplied.
// The database's unique email constraint also protects concurrent registrations.
func (r *Repository) Create(ctx context.Context, email, password string) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("user: create: %w", err)
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return 0, err
	}
	if err := validatePassword(password); err != nil {
		return 0, err
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return 0, fmt.Errorf("user: hash password: %w", err)
	}
	// Bcrypt itself cannot be interrupted; avoid an INSERT if the deadline passed
	// while the password was being hashed.
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("user: create: %w", err)
	}

	result, err := r.db.ExecContext(ctx,
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

// FindByEmail returns the account metadata for an email, or ErrNotFound.
// It applies the same email normalization as Create and never selects credentials.
func (r *Repository) FindByEmail(ctx context.Context, email string) (*User, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("user: find by email: %w", err)
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}

	var u User
	err = r.db.QueryRowContext(ctx,
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

func validatePassword(password string) error {
	if len(password) > MaxPasswordBytes {
		return fmt.Errorf("%w: must not exceed %d bytes", ErrInvalidPassword, MaxPasswordBytes)
	}
	if !utf8.ValidString(password) {
		return fmt.Errorf("%w: must be valid UTF-8", ErrInvalidPassword)
	}
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return fmt.Errorf("%w: must contain at least %d characters", ErrInvalidPassword, MinPasswordLength)
	}
	return nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
