package repo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"strings"

	"github.com/Emulisy/Supa-Agent/internal/model"
	"github.com/go-sql-driver/mysql"
)

type MemoryRepository struct{ db *sql.DB }

func NewMemoryRepository(db *sql.DB) (*MemoryRepository, error) {
	if db == nil {
		return nil, errors.New("memory: database is required")
	}
	return &MemoryRepository{db: db}, nil
}

func (r *MemoryRepository) List(ctx context.Context, scope model.MemoryScope, activeOnly bool, limit, offset int) ([]model.Memory, error) {
	query := `SELECT id, content, archived, created_at, updated_at, revision
		FROM agent_memories WHERE user_id = ? AND agent_id = ?`
	if activeOnly {
		query += ` AND archived = FALSE`
	}
	query += ` ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?`
	rows, err := r.db.QueryContext(ctx, query, scope.UserID, scope.AgentID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	memories := make([]model.Memory, 0)
	for rows.Next() {
		var m model.Memory
		if err := rows.Scan(&m.ID, &m.Content, &m.Archived, &m.CreatedAt, &m.UpdatedAt, &m.Revision); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

// Apply commits the batch together. Revisions prevent model decisions based on
// an older snapshot from overwriting a concurrent correction or archive.
func (r *MemoryRepository) Apply(ctx context.Context, scope model.MemoryScope, changes []model.MemoryChange) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, change := range changes {
		var err error
		switch change.Op {
		case "add":
			digest := memoryDigest(change.Content)
			_, err = tx.ExecContext(ctx, `INSERT INTO agent_memories (user_id, agent_id, content, active_digest) VALUES (?, ?, ?, ?)`,
				scope.UserID, scope.AgentID, change.Content, digest[:])
		case "update":
			digest := memoryDigest(change.Content)
			_, err = tx.ExecContext(ctx, `UPDATE agent_memories SET content = ?, active_digest = ?, revision = revision + 1
				WHERE user_id = ? AND agent_id = ? AND id = ? AND revision = ? AND archived = FALSE`,
				change.Content, digest[:], scope.UserID, scope.AgentID, change.ID, change.Revision)
		case "archive":
			_, err = tx.ExecContext(ctx, `UPDATE agent_memories SET archived = TRUE, active_digest = NULL, revision = revision + 1
				WHERE user_id = ? AND agent_id = ? AND id = ? AND revision = ? AND archived = FALSE`,
				scope.UserID, scope.AgentID, change.ID, change.Revision)
		default:
			return model.ErrInvalidInput
		}
		var duplicate *mysql.MySQLError
		if err != nil && !(errors.As(err, &duplicate) && duplicate.Number == 1062) {
			return err
		}
	}
	return tx.Commit()
}

func memoryDigest(content string) [32]byte {
	return sha256.Sum256([]byte(strings.ToLower(strings.Join(strings.Fields(content), " "))))
}

func (r *MemoryRepository) Archive(ctx context.Context, scope model.MemoryScope, id uint64) error {
	return memoryMutationResult(r.db.ExecContext(ctx, `UPDATE agent_memories SET archived = TRUE, active_digest = NULL, revision = revision + 1
		WHERE user_id = ? AND agent_id = ? AND id = ?`, scope.UserID, scope.AgentID, id))
}

func (r *MemoryRepository) Delete(ctx context.Context, scope model.MemoryScope, id uint64) error {
	return memoryMutationResult(r.db.ExecContext(ctx, `DELETE FROM agent_memories WHERE user_id = ? AND agent_id = ? AND id = ?`,
		scope.UserID, scope.AgentID, id))
}

func memoryMutationResult(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return model.ErrMemoryNotFound
	}
	return err
}
