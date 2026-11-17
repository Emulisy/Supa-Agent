package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Emulisy/Supa-Agent/internal/model"
	"github.com/Emulisy/Supa-Agent/internal/security"
	"golang.org/x/crypto/bcrypt"
)

// AuthResult contains the public account and its short-lived access token.
// ExpiresIn is the configured access-token lifetime in seconds.
type AuthResult struct {
	User        *model.User `json:"user"`
	AccessToken string      `json:"access_token"`
	TokenType   string      `json:"token_type"`
	ExpiresIn   int64       `json:"expires_in"`
}

// UserRepository is the account persistence needed by authentication.
type UserRepository interface {
	Create(context.Context, string, string) (uint64, error)
	FindByID(context.Context, uint64) (*model.User, error)
	FindByEmail(context.Context, string) (*model.User, error)
	FindCredentialsByEmail(context.Context, string) (uint64, string, error)
}
type TokenIssuer interface {
	Sign(uint64) (string, error)
	TTL() time.Duration
}
type AuthService struct {
	users             UserRepository
	tokens            TokenIssuer
	dummyPasswordHash []byte
}

func NewAuthService(users UserRepository, tokens TokenIssuer) (*AuthService, error) {
	if users == nil || tokens == nil {
		return nil, errors.New("auth: repository and token issuer are required")
	}
	dummy, err := security.HashPassword(rand.Text())
	if err != nil {
		return nil, err
	}
	return &AuthService{users: users, tokens: tokens, dummyPasswordHash: []byte(dummy)}, nil
}
func (s *AuthService) UserByID(ctx context.Context, id uint64) (*model.User, error) {
	return s.users.FindByID(ctx, id)
}

// Register validates account input, stores a password hash, and signs the user in.
// Input and duplicate-email errors remain identifiable with errors.Is.
func (s *AuthService) Register(ctx context.Context, email, password string) (*AuthResult, error) {
	if s == nil || s.users == nil {
		return nil, errors.New("auth: service is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("auth: register: %w", err)
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		return nil, err
	}
	// Bcrypt cannot be interrupted; check the deadline before creating an account.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("auth: register: %w", err)
	}
	userID, err := s.users.Create(ctx, email, passwordHash)
	if err != nil {
		return nil, fmt.Errorf("auth: register: %w", err)
	}
	return s.authResult(ctx, userID, email)
}

// Login verifies stored credentials before issuing an access token.
// Registration's minimum password length is not reapplied during login, so a
// future password-policy change does not lock existing users out.
func (s *AuthService) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	if s == nil || s.users == nil {
		return nil, errors.New("auth: service is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("auth: login: %w", err)
	}
	email, err := normalizeEmail(email)
	if err != nil || len(password) > security.MaxPasswordBytes || !utf8.ValidString(password) {
		return nil, model.ErrInvalidCredentials
	}
	userID, passwordHash, err := s.users.FindCredentialsByEmail(ctx, email)
	if errors.Is(err, model.ErrUserNotFound) {
		_ = bcrypt.CompareHashAndPassword(s.dummyPasswordHash, []byte(password))
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("auth: login: %w", err)
		}
		return nil, model.ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: load credentials: %w", err)
	}
	err = bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, fmt.Errorf("auth: login: %w", contextErr)
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return nil, model.ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: verify stored password: %w", err)
	}
	return s.authResult(ctx, userID, email)
}

func (s *AuthService) authResult(ctx context.Context, userID uint64, email string) (*AuthResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("auth: load account: %w", err)
	}
	u, err := s.users.FindByEmail(ctx, email)
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
	token, err := s.tokens.Sign(userID)
	if err != nil {
		return nil, fmt.Errorf("auth: issue access token: %w", err)
	}
	return &AuthResult{
		User:        u,
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(s.tokens.TTL() / time.Second),
	}, nil
}

func normalizeEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" || len(email) > 254 || !utf8.ValidString(email) {
		return "", model.ErrInvalidEmail
	}
	email = strings.ToLower(email)
	if len(email) > 254 {
		return "", model.ErrInvalidEmail
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Name != "" || address.Address != email {
		return "", model.ErrInvalidEmail
	}
	return email, nil
}
