package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// DefaultAccessTokenTTL is the recommended lifetime for access JWTs.
	DefaultAccessTokenTTL = 15 * time.Minute

	jwtIssuer   = "supa-agent"
	jwtAudience = "supa-agent-api"
)

// ErrInvalidToken indicates that an access JWT failed verification or validation.
var ErrInvalidToken = errors.New("auth: invalid access token")

// JWTManager signs and validates access JWTs without database or session lookups.
// Construct it once at startup and share it between authentication handlers.
type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

// NewJWTManager requires a secret containing at least 32 cryptographically random
// bytes and a TTL of at least one second, the precision used for JWT timestamps.
func NewJWTManager(secret string, ttl time.Duration) (*JWTManager, error) {
	manager := &JWTManager{secret: []byte(secret), ttl: ttl}
	if err := manager.validateConfig(); err != nil {
		return nil, err
	}
	return manager, nil
}

// GenerateJWT creates an HS256 access token for an authenticated user's ID.
// Call it only after successfully verifying the user's credentials.
func (m *JWTManager) GenerateJWT(userID uint64) (string, error) {
	if err := m.validateConfig(); err != nil {
		return "", err
	}
	if userID == 0 {
		return "", errors.New("auth: JWT user ID must be positive")
	}
	now := time.Now().UTC()

	claims := jwt.RegisteredClaims{
		Issuer:    jwtIssuer,
		Subject:   strconv.FormatUint(userID, 10),
		Audience:  jwt.ClaimStrings{jwtAudience},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		ID:        rand.Text(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign JWT: %w", err)
	}
	return signed, nil
}

// ValidateJWT verifies a raw token string and returns its authenticated user ID.
// It requires HS256, the app's issuer and audience, expiration, issued-at, and a
// positive uint64 subject. Callers can identify failures with ErrInvalidToken and
// specific causes such as jwt.ErrTokenExpired using errors.Is.
func (m *JWTManager) ValidateJWT(tokenString string) (uint64, error) {
	if err := m.validateConfig(); err != nil {
		return 0, err
	}
	var claims jwt.RegisteredClaims
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(_ *jwt.Token) (any, error) {
		return m.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(jwtIssuer),
		jwt.WithAudience(jwtAudience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithStrictDecoding(),
	)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if token == nil || !token.Valid {
		return 0, ErrInvalidToken
	}
	if claims.IssuedAt == nil {
		return 0, fmt.Errorf("%w: issued-at claim is required", ErrInvalidToken)
	}
	userID, err := strconv.ParseUint(claims.Subject, 10, 64)
	if err != nil || userID == 0 || strconv.FormatUint(userID, 10) != claims.Subject {
		return 0, fmt.Errorf("%w: subject must be a positive uint64 user ID", ErrInvalidToken)
	}
	return userID, nil
}

func (m *JWTManager) validateConfig() error {
	if m == nil {
		return errors.New("auth: JWT manager is required")
	}
	if len(m.secret) < 32 {
		return errors.New("auth: JWT secret must be at least 32 bytes")
	}
	if m.ttl < time.Second {
		return errors.New("auth: JWT TTL must be at least one second")
	}
	return nil
}
