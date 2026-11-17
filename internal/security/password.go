package security

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	MinPasswordLength = 8
	MaxPasswordBytes  = 72
)

// ErrInvalidPassword indicates that a password does not meet the input policy.
var ErrInvalidPassword = errors.New("auth: invalid password")

// HashPassword validates a plaintext password and returns its bcrypt hash.
// Passwords are used exactly as supplied, without trimming or normalization.
func HashPassword(password string) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(hash), nil
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
