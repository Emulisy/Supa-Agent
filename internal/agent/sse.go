package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"
)

// RunEventType identifies an event stored in Redis and delivered over SSE.
type RunEventType string

const (
	RunEventStarted      RunEventType = "run_started"
	RunEventMessageDelta RunEventType = "message_delta"
	RunEventMessage      RunEventType = "message"
	RunEventMessageEnd   RunEventType = "message_end"
	RunEventError        RunEventType = "error"
	RunEventDone         RunEventType = "done"
)

// RunEvent is the serializable representation of an agent run event. The event
// consumer supplies RunID, AgentID, and a sequential MessageID for each outer
// message output. ID is assigned by Redis and becomes the SSE event ID.
type RunEvent struct {
	ID        string          `json:"id,omitempty"`
	Type      RunEventType    `json:"type"`
	RunID     string          `json:"run_id,omitempty"`
	AgentID   uint64          `json:"agent_id,omitempty"`
	MessageID uint64          `json:"message_id,omitempty"`
	AgentName string          `json:"agent_name,omitempty"`
	RunPath   []string        `json:"run_path,omitempty"`
	Role      schema.RoleType `json:"role,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	Message   *schema.Message `json:"message,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// Terminal reports whether the entire run has finished.
func (e RunEvent) Terminal() bool {
	return e.Type == RunEventError || e.Type == RunEventDone
}

// RunEventFromAgentEvent extracts message metadata and complete messages from
// Eino. Nil and events without message output or an error have no representation.
//
// For streaming output it returns a message_delta template with no Message. The
// consumer must read MessageStream and attach each chunk before publishing a copy
// of the template. This function neither reads nor closes the source stream.
// Run lifecycle events and message_end are emitted separately by that consumer;
// an individual agent's exit action does not imply that the whole run is done.
func RunEventFromAgentEvent(source *adk.AgentEvent) *RunEvent {
	if source == nil {
		return nil
	}
	event := &RunEvent{AgentName: source.AgentName}
	if len(source.RunPath) > 0 {
		event.RunPath = make([]string, len(source.RunPath))
		for i := range source.RunPath {
			event.RunPath[i] = source.RunPath[i].String()
		}
	}
	if source.Err != nil {
		event.Type = RunEventError
		event.Error = "agent execution failed"
		return event
	}
	if source.Output == nil || source.Output.MessageOutput == nil {
		return nil
	}
	output := source.Output.MessageOutput
	event.Role = output.Role
	event.ToolName = output.ToolName
	if output.IsStreaming {
		event.Type = RunEventMessageDelta
		return event
	}
	if output.Message == nil {
		return nil
	}
	event.Type = RunEventMessage
	event.Message = output.Message
	if event.Role == "" {
		event.Role = output.Message.Role
	}
	if event.ToolName == "" {
		event.ToolName = output.Message.ToolName
	}
	return event
}

// Append and expiration are atomic. Checking the last entry also prevents a
// second producer from appending events after this run has terminated.
var appendRunEventScript = redis.NewScript(`
local last = redis.call('XREVRANGE', KEYS[1], '+', '-', 'COUNT', 1)
if #last > 0 then
  local fields = last[1][2]
  for i = 1, #fields, 2 do
    if fields[i] == 'type' and (fields[i + 1] == 'done' or fields[i + 1] == 'error') then
      return redis.error_reply('RUN_FINISHED')
    end
  end
end
local id = redis.call('XADD', KEYS[1], '*', 'type', ARGV[1], 'data', ARGV[2])
redis.call('EXPIRE', KEYS[1], ARGV[3])
return id
`)

// WriteRunEvents consumes an Eino runner's iterator and writes each unpacked
// RunEvent to agent:runs:{runID}:events as type and JSON data fields. Redis assigns
// the entry IDs; a reader supplies those IDs when decoding RunEvent. Each append
// refreshes a one-hour TTL, without trimming earlier events. Call once per run
// with a unique runID.
//
// Nested message streams are read chunk by chunk and closed before advancing the
// iterator. Only successful iterator exhaustion emits done. Execution failures
// emit a sanitized error event and are also returned to the caller. On error the
// caller must cancel the context used to start the runner to stop upstream work.
// The caller owns the Redis client. No overall run timeout is added here.
func WriteRunEvents(ctx context.Context, client *redis.Client, runID string, agentID uint64, events *adk.AsyncIterator[*adk.AgentEvent]) (err error) {
	if client == nil || events == nil {
		return errors.New("agent: Redis client and event iterator are required")
	}
	if strings.TrimSpace(runID) == "" || strings.ContainsAny(runID, "{}") || agentID == 0 {
		return errors.New("agent: valid run ID and agent ID are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stream := "agent:runs:{" + runID + "}:events"
	write := func(writeCtx context.Context, event RunEvent) error {
		event.ID = ""
		event.RunID = runID
		event.AgentID = agentID
		data, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("agent: encode run event: %w", err)
		}
		writeCtx, cancel := context.WithTimeout(writeCtx, 5*time.Second)
		defer cancel()
		if err := appendRunEventScript.Run(writeCtx, client, []string{stream}, string(event.Type), string(data), int64(time.Hour/time.Second)).Err(); err != nil {
			return fmt.Errorf("agent: append run event: %w", err)
		}
		return nil
	}
	if err := write(ctx, RunEvent{Type: RunEventStarted}); err != nil {
		return err
	}
	var last RunEvent
	defer func() {
		if err == nil {
			return
		}
		last.ID = ""
		last.Type = RunEventError
		last.Message = nil
		last.Error = "agent execution failed"
		// Cancellation must still get a bounded opportunity to record failure.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if writeErr := write(cleanupCtx, last); writeErr != nil {
			err = errors.Join(err, writeErr)
		}
	}()
	var messageID uint64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		source, ok := events.Next()
		if !ok {
			break
		}
		if source == nil {
			continue
		}
		event := RunEventFromAgentEvent(source)
		if event != nil {
			if event.Type == RunEventMessage || event.Type == RunEventMessageDelta {
				messageID++
				event.MessageID = messageID
			}
			last = *event
		}
		if err := writeAgentRunEvent(ctx, source, event, func(event RunEvent) error { return write(ctx, event) }); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return write(ctx, RunEvent{Type: RunEventDone})
}

//write the run event to redis stream
func writeAgentRunEvent(ctx context.Context, source *adk.AgentEvent, event *RunEvent, write func(RunEvent) error) error {
	var output *adk.MessageVariant
	if source.Output != nil {
		output = source.Output.MessageOutput
	}
	if output != nil && output.MessageStream != nil {
		var once sync.Once
		closeStream := func() { once.Do(output.MessageStream.Close) }
		stop := context.AfterFunc(ctx, closeStream)
		defer stop()
		defer closeStream()
	}
	if source.Err != nil {
		return source.Err
	}
	if source.Action != nil && source.Action.Interrupted != nil {
		return errors.New("agent: interrupted runs cannot be resumed")
	}
	if output == nil {
		return nil
	}
	if event == nil {
		return errors.New("agent: missing message output")
	}
	if !output.IsStreaming {
		return write(*event)
	}
	if output.MessageStream == nil {
		return errors.New("agent: missing message stream")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		message, err := output.MessageStream.Recv()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, io.EOF) {
			end := *event
			end.Type = RunEventMessageEnd
			end.Message = nil
			return write(end)
		}
		if err != nil {
			return err
		}
		if message == nil {
			continue
		}
		if event.Role == "" {
			event.Role = message.Role
		}
		if event.ToolName == "" {
			event.ToolName = message.ToolName
		}
		delta := *event
		delta.Message = message
		if err := write(delta); err != nil {
			return err
		}
	}
}
