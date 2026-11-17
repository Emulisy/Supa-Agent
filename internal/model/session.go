package model

import (
	"time"

	"github.com/cloudwego/eino/schema"
)

type SessionInfo struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Title     string    `json:"title"`
}

type Session struct {
	SessionInfo
	Messages []*schema.Message `json:"messages"`
}
