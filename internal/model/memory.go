package model

import "time"

// MemoryScope is derived from an authenticated, owned agent definition.
type MemoryScope struct {
	UserID  uint64
	AgentID uint64
}

type Memory struct {
	ID        uint64    `json:"id"`
	Content   string    `json:"content"`
	Archived  bool      `json:"archived"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Revision  uint64    `json:"-"`
}

type MemoryChange struct {
	Op       string
	ID       uint64
	Revision uint64
	Content  string
}
