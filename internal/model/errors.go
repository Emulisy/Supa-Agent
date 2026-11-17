package model

import "errors"

const MaxPageSize = 100

var (
	ErrUserNotFound       = errors.New("user: not found")
	ErrInvalidEmail       = errors.New("user: invalid email address")
	ErrEmailExists        = errors.New("user: email already exists")
	ErrInvalidCredentials = errors.New("auth: invalid email or password")
	ErrAgentNotFound      = errors.New("agent: definition not found")
	ErrInvalidDefinition  = errors.New("agent: invalid definition")
	ErrInvalidPagination  = errors.New("agent: invalid pagination")
	ErrSessionNotFound    = errors.New("session: not found")
	ErrSessionDeleted     = errors.New("memory: session has been deleted")
	ErrRunNotFound        = errors.New("run: not found")
	ErrRunFinished        = errors.New("run: already finished")
	ErrConflict           = errors.New("session: a run is active")
	ErrCapacity           = errors.New("run: capacity reached")
	ErrInvalidInput       = errors.New("invalid input")
	ErrUnavailable        = errors.New("service unavailable")
)
