package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Emulisy/Supa-Agent/internal/model"
	"github.com/go-sql-driver/mysql"
)

type UserRepository struct{ db *sql.DB }

func NewUserRepository(db *sql.DB) (*UserRepository, error) {
	if db == nil {
		return nil, errors.New("user: database is required")
	}
	return &UserRepository{db: db}, nil
}

// FindByID loads public account data. Callers supply an authorized user ID.
func (s *UserRepository) FindByID(ctx context.Context, userID uint64) (*model.User, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("auth: service is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("user: find by ID: %w", err)
	}
	if userID == 0 {
		return nil, model.ErrUserNotFound
	}
	var u model.User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, created_at, updated_at FROM users WHERE id = ?`,
		userID,
	).Scan(&u.ID, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("user: find by ID: %w", model.ErrUserNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("user: find by ID: %w", err)
	}
	return &u, nil
}

// Create persists the normalized email and password hash prepared by Register.
// The database's unique email constraint protects concurrent registrations.
func (s *UserRepository) Create(ctx context.Context, email, passwordHash string) (uint64, error) {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO users (email, password_hash) VALUES (?, ?)`,
		email, passwordHash,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			// MySQL's duplicate-entry message contains the submitted email. Expose
			// only the domain error rather than including that message.
			return 0, fmt.Errorf("user: create: %w", model.ErrEmailExists)
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

// FindByEmail reads public account data for an already-normalized email.
func (s *UserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var u model.User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, created_at, updated_at FROM users WHERE email = ?`,
		email,
	).Scan(&u.ID, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("user: find by email: %w", model.ErrUserNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("user: find by email: %w", err)
	}
	return &u, nil
}

// FindCredentialsByEmail keeps password hashes separate from public User values.
func (s *UserRepository) FindCredentialsByEmail(ctx context.Context, email string) (uint64, string, error) {
	var userID uint64
	var passwordHash string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, password_hash FROM users WHERE email = ?`,
		email,
	).Scan(&userID, &passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", fmt.Errorf("user: find credentials by email: %w", model.ErrUserNotFound)
	}
	if err != nil {
		return 0, "", fmt.Errorf("user: find credentials by email: %w", err)
	}
	return userID, passwordHash, nil
}
