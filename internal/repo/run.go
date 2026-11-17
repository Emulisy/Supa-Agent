package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Emulisy/Supa-Agent/internal/model"
	"github.com/redis/go-redis/v9"
)

const RunRetention = time.Hour

var runIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type RunRepository struct{ client *redis.Client }

func NewRunRepository(client *redis.Client) (*RunRepository, error) {
	if client == nil {
		return nil, errors.New("run: Redis client is required")
	}
	return &RunRepository{client: client}, nil
}

func runKeys(id string) (string, string, error) {
	if !runIDPattern.MatchString(id) {
		return "", "", model.ErrRunNotFound
	}
	return "agent:runs:{" + id + "}:meta", "agent:runs:{" + id + "}:events", nil
}

var createRunScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1], KEYS[2]) > 0 then return redis.error_reply('RUN_EXISTS') end
redis.call('HSET', KEYS[1], 'run_id', ARGV[1], 'owner', ARGV[2], 'agent_id', ARGV[3], 'session_id', ARGV[4], 'status', 'running', 'events_url', ARGV[5])
local id = redis.call('XADD', KEYS[2], '*', 'type', 'run_started', 'data', ARGV[6])
redis.call('EXPIRE', KEYS[1], ARGV[7])
redis.call('EXPIRE', KEYS[2], ARGV[7])
return id
`)

// Terminal state and the final stream entry are committed by the same script.
var appendRunScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 or redis.call('EXISTS', KEYS[2]) == 0 then return redis.error_reply('RUN_NOT_FOUND') end
if redis.call('HGET', KEYS[1], 'status') ~= 'running' then return redis.error_reply('RUN_FINISHED') end
local id = redis.call('XADD', KEYS[2], '*', 'type', ARGV[1], 'data', ARGV[2])
if ARGV[3] ~= '' then redis.call('HSET', KEYS[1], 'status', ARGV[3]) end
redis.call('EXPIRE', KEYS[1], ARGV[4])
redis.call('EXPIRE', KEYS[2], ARGV[4])
return id
`)

func redisError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if strings.Contains(err.Error(), "RUN_NOT_FOUND") {
		return model.ErrRunNotFound
	}
	if strings.Contains(err.Error(), "RUN_FINISHED") {
		return model.ErrRunFinished
	}
	return fmt.Errorf("%w: Redis: %v", model.ErrUnavailable, err)
}

func (r *RunRepository) Create(ctx context.Context, run model.Run) error {
	meta, stream, err := runKeys(run.ID)
	if err != nil {
		return err
	}
	if run.UserID == 0 || run.AgentID == 0 || run.SessionID == "" || run.Status != model.RunRunning {
		return model.ErrInvalidInput
	}
	data, err := json.Marshal(model.RunEvent{Type: model.RunEventStarted, RunID: run.ID, AgentID: run.AgentID})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return redisError(createRunScript.Run(ctx, r.client, []string{meta, stream}, run.ID, run.UserID, run.AgentID, run.SessionID, run.EventsURL, string(data), int64(RunRetention/time.Second)).Err())
}

func (r *RunRepository) Get(ctx context.Context, userID uint64, id string) (*model.Run, error) {
	meta, _, err := runKeys(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	values, err := r.client.HGetAll(ctx, meta).Result()
	if err != nil {
		return nil, redisError(err)
	}
	if len(values) == 0 {
		return nil, model.ErrRunNotFound
	}
	owner, err := strconv.ParseUint(values["owner"], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid run owner", model.ErrUnavailable)
	}
	if userID == 0 || owner != userID {
		return nil, model.ErrRunNotFound
	}
	agentID, err := strconv.ParseUint(values["agent_id"], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid run agent", model.ErrUnavailable)
	}
	return &model.Run{ID: id, UserID: owner, AgentID: agentID, SessionID: values["session_id"], Status: model.RunStatus(values["status"]), EventsURL: values["events_url"]}, nil
}

func (r *RunRepository) Append(ctx context.Context, run model.Run, event model.RunEvent) error {
	if event.Terminal() || event.Type == model.RunEventStarted {
		return model.ErrInvalidInput
	}
	return r.append(ctx, run, event, "")
}

func (r *RunRepository) Finish(ctx context.Context, run model.Run, status model.RunStatus, message string) error {
	event := model.RunEvent{Type: model.RunEventError, Error: message}
	switch status {
	case model.RunSucceeded:
		event.Type, event.Error = model.RunEventDone, ""
	case model.RunFailed, model.RunCanceled:
	default:
		return model.ErrInvalidInput
	}
	return r.append(ctx, run, event, status)
}

func (r *RunRepository) append(ctx context.Context, run model.Run, event model.RunEvent, status model.RunStatus) error {
	meta, stream, err := runKeys(run.ID)
	if err != nil {
		return err
	}
	event.ID, event.RunID, event.AgentID = "", run.ID, run.AgentID
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return redisError(appendRunScript.Run(ctx, r.client, []string{meta, stream}, string(event.Type), string(data), string(status), int64(RunRetention/time.Second)).Err())
}

func (r *RunRepository) Read(ctx context.Context, id, after string, block time.Duration) ([]model.RunEvent, error) {
	_, stream, err := runKeys(id)
	if err != nil {
		return nil, err
	}
	result, err := r.client.XRead(ctx, &redis.XReadArgs{Streams: []string{stream, after}, Count: 100, Block: block}).Result()
	if errors.Is(err, redis.Nil) {
		return []model.RunEvent{}, nil
	}
	if err != nil {
		return nil, redisError(err)
	}
	events := make([]model.RunEvent, 0)
	for _, batch := range result {
		for _, entry := range batch.Messages {
			var event model.RunEvent
			data, ok := entry.Values["data"].(string)
			if !ok {
				return nil, errors.New("run: invalid event record")
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				return nil, err
			}
			event.ID = entry.ID
			events = append(events, event)
		}
	}
	return events, nil
}

// Recover is called before accepting requests. This deployment has one API process.
func (r *RunRepository) Recover(ctx context.Context) error {
	var cursor uint64
	for {
		keys, next, err := r.client.Scan(ctx, cursor, "agent:runs:*:meta", 100).Result()
		if err != nil {
			return redisError(err)
		}
		for _, key := range keys {
			values, err := r.client.HGetAll(ctx, key).Result()
			if err != nil {
				return redisError(err)
			}
			if values["status"] != string(model.RunRunning) {
				continue
			}
			owner, err := strconv.ParseUint(values["owner"], 10, 64)
			if err != nil {
				return errors.New("run: invalid recovery metadata")
			}
			run, err := r.Get(ctx, owner, values["run_id"])
			if errors.Is(err, model.ErrRunNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if err := r.Finish(ctx, *run, model.RunFailed, "server restarted"); err != nil && !errors.Is(err, model.ErrRunFinished) && !errors.Is(err, model.ErrRunNotFound) {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}
