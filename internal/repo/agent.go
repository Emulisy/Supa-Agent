package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Emulisy/Supa-Agent/internal/model"
)

// AgentRepository persists Definitions using a caller-owned MySQL connection pool.
// Share the repository between requests; callers own the model.Definition values.
type AgentRepository struct {
	db *sql.DB
}

// NewAgentRepository reuses an existing pool, such as the result of infra.OpenMySQL.
// The application remains responsible for closing the pool.
func NewAgentRepository(db *sql.DB) (*AgentRepository, error) {
	if db == nil {
		return nil, errors.New("agent: database is required")
	}
	return &AgentRepository{db: db}, nil
}

// Save inserts a new definition and assigns its generated ID on success.
// Its ID must be zero and UserID must identify the authenticated owner.
// Use Update to modify an existing definition; Save never performs an upsert.
func (r *AgentRepository) Save(ctx context.Context, definition *model.Definition) error {
	if err := r.ready(ctx); err != nil {
		return fmt.Errorf("agent: save: %w", err)
	}
	if err := model.ValidateDefinition(definition); err != nil {
		return err
	}
	if definition.ID != 0 {
		return fmt.Errorf("%w: new definitions must not have an ID", model.ErrInvalidDefinition)
	}

	result, err := r.db.ExecContext(ctx,
		`INSERT INTO agents (user_id, type, config) VALUES (?, ?, ?)`,
		definition.UserID, string(definition.Type), string(definition.Config),
	)
	if err != nil {
		return fmt.Errorf("agent: save: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("agent: read inserted ID: %w", err)
	}
	if id == 0 {
		return errors.New("agent: database returned an invalid inserted ID")
	}
	// The MySQL driver represents unsigned BIGINT insert IDs as int64.
	definition.ID = uint64(id)
	return nil
}

// Update replaces the type and config of an existing definition owned by UserID.
// It never changes ownership. Unchanged values are a successful update.
func (r *AgentRepository) Update(ctx context.Context, definition *model.Definition) error {
	if err := r.ready(ctx); err != nil {
		return fmt.Errorf("agent: update: %w", err)
	}
	if err := model.ValidateDefinition(definition); err != nil {
		return err
	}
	if definition.ID == 0 {
		return fmt.Errorf("%w: an ID is required for updates", model.ErrInvalidDefinition)
	}

	result, err := r.db.ExecContext(ctx,
		`UPDATE agents SET type = ?, config = ? WHERE user_id = ? AND id = ?`,
		string(definition.Type), string(definition.Config), definition.UserID, definition.ID,
	)
	if err != nil {
		return fmt.Errorf("agent: update: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("agent: update affected rows: %w", err)
	}
	if affected != 0 {
		return nil
	}

	// MySQL normally reports changed rows, not matched rows. A no-op update may
	// therefore report zero even though the owned definition exists.
	var exists bool
	err = r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM agents WHERE user_id = ? AND id = ?)`,
		definition.UserID, definition.ID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("agent: check updated definition: %w", err)
	}
	if !exists {
		return fmt.Errorf("agent: update: %w", model.ErrAgentNotFound)
	}
	return nil
}

// Delete removes a definition belonging to the authenticated userID.
// A missing definition or a different owner produces model.ErrAgentNotFound.
func (r *AgentRepository) Delete(ctx context.Context, userID, id uint64) error {
	if err := r.ready(ctx); err != nil {
		return fmt.Errorf("agent: delete: %w", err)
	}
	if err := validateIdentity(userID, id); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM agents WHERE user_id = ? AND id = ?`, userID, id,
	)
	if err != nil {
		return fmt.Errorf("agent: delete: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("agent: delete affected rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("agent: delete: %w", model.ErrAgentNotFound)
	}
	return nil
}

// FindByID loads a definition belonging to the authenticated userID.
// A missing definition or a different owner produces model.ErrAgentNotFound.
func (r *AgentRepository) FindByID(ctx context.Context, userID, id uint64) (*model.Definition, error) {
	if err := r.ready(ctx); err != nil {
		return nil, fmt.Errorf("agent: find by ID: %w", err)
	}
	if err := validateIdentity(userID, id); err != nil {
		return nil, err
	}
	definition, err := scanDefinition(r.db.QueryRowContext(ctx,
		`SELECT id, user_id, type, config FROM agents WHERE user_id = ? AND id = ?`,
		userID, id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("agent: find by ID: %w", model.ErrAgentNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("agent: find by ID: %w", err)
	}
	return definition, nil
}

// ListByUserID loads the authenticated owner's definitions in ascending ID order.
// Limit must be 1..model.MaxPageSize and offset must be non-negative. An empty page is
// returned as an empty slice rather than nil.
func (r *AgentRepository) ListByUserID(ctx context.Context, userID uint64, limit, offset int) ([]model.Definition, error) {
	if err := r.ready(ctx); err != nil {
		return nil, fmt.Errorf("agent: list: %w", err)
	}
	if userID == 0 {
		return nil, fmt.Errorf("%w: user ID is required", model.ErrInvalidDefinition)
	}
	if limit < 1 || limit > model.MaxPageSize || offset < 0 {
		return nil, fmt.Errorf("%w: limit must be 1..%d and offset must be non-negative", model.ErrInvalidPagination, model.MaxPageSize)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, type, config FROM agents WHERE user_id = ? ORDER BY id ASC LIMIT ? OFFSET ?`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("agent: list: %w", err)
	}
	defer rows.Close()

	definitions := make([]model.Definition, 0)
	for rows.Next() {
		definition, err := scanDefinition(rows)
		if err != nil {
			return nil, fmt.Errorf("agent: scan list: %w", err)
		}
		definitions = append(definitions, *definition)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("agent: read list: %w", err)
	}
	return definitions, nil
}

func (r *AgentRepository) ready(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("agent: repository is not initialized")
	}
	return ctx.Err()
}

func validateIdentity(userID, id uint64) error {
	if userID == 0 || id == 0 {
		return fmt.Errorf("%w: user ID and definition ID are required", model.ErrInvalidDefinition)
	}
	return nil
}

func scanDefinition(row interface{ Scan(...any) error }) (*model.Definition, error) {
	var definition model.Definition
	var config []byte
	if err := row.Scan(&definition.ID, &definition.UserID, &definition.Type, &config); err != nil {
		return nil, err
	}
	definition.Config = json.RawMessage(config)
	return &definition, nil
}
