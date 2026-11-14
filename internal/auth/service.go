package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials is returned for an unknown email or an incorrect password.
var ErrInvalidCredentials = errors.New("auth: invalid email or password")

// AuthResult contains the public account and its short-lived access token.
// ExpiresIn is the configured access-token lifetime in seconds.
type AuthResult struct {
	User        *User  `json:"user"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

// Service owns authentication logic and signing state. Construct it with
// NewService and share it between handlers. Its caller owns the database pool.
type Service struct {
	db                *sql.DB
	secret            []byte
	ttl               time.Duration
	dummyPasswordHash []byte
}

// NewService validates the database and signing configuration once at startup.
// The secret must contain at least 32 cryptographically random bytes, and the
// TTL must be at least one second. DefaultAccessTokenTTL is 15 minutes.
func NewService(db *sql.DB, secret string, ttl time.Duration) (*Service, error) {
	if db == nil {
		return nil, errors.New("auth: database is required")
	}
	if len(secret) < 32 {
		return nil, errors.New("auth: JWT secret must be at least 32 bytes")
	}
	if ttl < time.Second {
		return nil, errors.New("auth: JWT TTL must be at least one second")
	}
	// Missing accounts still perform a bcrypt comparison at the normal cost.
	// Generate this hash once at startup rather than on every failed login.
	dummyHash, err := HashPassword(rand.Text())
	if err != nil {
		return nil, fmt.Errorf("auth: initialize password verification: %w", err)
	}
	return &Service{
		db:                db,
		secret:            []byte(secret),
		ttl:               ttl,
		dummyPasswordHash: []byte(dummyHash),
	}, nil
}

// Register validates account input, stores a password hash, and signs the user in.
// Input and duplicate-email errors remain identifiable with errors.Is.
func (s *Service) Register(ctx context.Context, email, password string) (*AuthResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("auth: service is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("auth: register: %w", err)
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	// Bcrypt cannot be interrupted; check the deadline before creating an account.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("auth: register: %w", err)
	}
	userID, err := s.createUser(ctx, email, passwordHash)
	if err != nil {
		return nil, fmt.Errorf("auth: register: %w", err)
	}
	return s.authResult(ctx, userID, email)
}

// Login verifies stored credentials before issuing an access token.
// Registration's minimum password length is not reapplied during login, so a
// future password-policy change does not lock existing users out.
func (s *Service) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("auth: service is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("auth: login: %w", err)
	}
	email, err := normalizeEmail(email)
	if err != nil || len(password) > MaxPasswordBytes || !utf8.ValidString(password) {
		return nil, ErrInvalidCredentials
	}
	userID, passwordHash, err := s.findCredentialsByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword(s.dummyPasswordHash, []byte(password))
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("auth: login: %w", err)
		}
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: load credentials: %w", err)
	}
	err = bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, fmt.Errorf("auth: login: %w", contextErr)
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: verify stored password: %w", err)
	}
	return s.authResult(ctx, userID, email)
}

func (s *Service) authResult(ctx context.Context, userID uint64, email string) (*AuthResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("auth: load account: %w", err)
	}
	u, err := s.findByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("auth: load account: %w", err)
	}
	// An email may have been changed or reassigned between database operations.
	// Issue tokens only for the account that was created or password-verified.
	if u.ID != userID {
		return nil, errors.New("auth: account identity changed during authentication")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("auth: issue access token: %w", err)
	}
	token, err := s.generateJWT(userID)
	if err != nil {
		return nil, fmt.Errorf("auth: issue access token: %w", err)
	}
	return &AuthResult{
		User:        u,
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(s.ttl / time.Second),
	}, nil
}
