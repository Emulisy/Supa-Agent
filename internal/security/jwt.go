package security

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

// JWTManager owns signing keys, independently of account storage.
type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

func NewJWTManager(secret string, ttl time.Duration) (*JWTManager, error) {
	if len(secret) < 32 {
		return nil, errors.New("auth: JWT secret must be at least 32 bytes")
	}
	if ttl < time.Second {
		return nil, errors.New("auth: JWT TTL must be at least one second")
	}
	return &JWTManager{secret: []byte(secret), ttl: ttl}, nil
}
func (s *JWTManager) TTL() time.Duration { return s.ttl }

// Sign is called after account creation or successful password verification.
func (s *JWTManager) Sign(userID uint64) (string, error) {
	if s == nil || len(s.secret) < 32 {
		return "", errors.New("auth: JWT manager is not initialized")
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
		ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		ID:        rand.Text(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign JWT: %w", err)
	}
	return signed, nil
}

// Validate verifies a raw token string and returns its authenticated user ID,
// without database or session lookups.
// It requires HS256, the app's issuer and audience, expiration, issued-at, and a
// positive uint64 subject. Callers can identify failures with ErrInvalidToken and
// specific causes such as jwt.ErrTokenExpired using errors.Is.
func (s *JWTManager) Validate(tokenString string) (uint64, error) {
	// A zero-value service must never accept tokens signed with an empty HMAC key.
	if s == nil || len(s.secret) < 32 {
		return 0, errors.New("auth: service is not initialized")
	}
	var claims jwt.RegisteredClaims
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(_ *jwt.Token) (any, error) {
		return s.secret, nil
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
