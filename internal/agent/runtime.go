// Package agent implements Eino execution without persistence or HTTP dependencies.
package agent

import (
	"context"
	"errors"
	"io"
	"sync"

	domain "github.com/Emulisy/Supa-Agent/internal/model"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type ModelResolver interface {
	ValidateModel(string) error
	Model(string) (model.ToolCallingChatModel, error)
}

// Execution is prepared before a run is accepted, without calling the provider.
type Execution interface {
	Run(context.Context, []*schema.Message, func(domain.RunEvent) error) ([]*schema.Message, error)
}

type Runtime struct{ models ModelResolver }

func NewRuntime(models ModelResolver) (*Runtime, error) {
	if chatModelDependencyMissing(models) {
		return nil, errors.New("agent: model resolver is required")
	}
	return &Runtime{models: models}, nil
}

func (r *Runtime) Validate(ctx context.Context, def *domain.Definition) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a, err := NewChatModelAgent(def)
	if err != nil {
		return err
	}
	cfg, err := a.configuration()
	if err != nil {
		return err
	}
	return r.models.ValidateModel(cfg.Model)
}

func (r *Runtime) Prepare(ctx context.Context, def *domain.Definition) (Execution, error) {
	if err := r.Validate(ctx, def); err != nil {
		return nil, err
	}
	a, err := NewChatModelAgent(def)
	if err != nil {
		return nil, err
	}
	cfg, err := a.configuration()
	if err != nil {
		return nil, err
	}
	m, err := r.models.Model(cfg.Model)
	if err != nil {
		return nil, err
	}
	built, err := a.Build(ctx, m)
	if err != nil {
		return nil, err
	}
	return &execution{agent: built}, nil
}

type execution struct{ agent adk.Agent }

func (e *execution) Run(ctx context.Context, history []*schema.Message, emit func(domain.RunEvent) error) ([]*schema.Message, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: e.agent, EnableStreaming: true})
	return consume(ctx, runner.Run(ctx, history), emit)
}

func consume(ctx context.Context, events *adk.AsyncIterator[*adk.AgentEvent], emit func(domain.RunEvent) error) ([]*schema.Message, error) {
	var completed []*schema.Message
	var messageID uint64
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		source, ok := events.Next()
		if !ok {
			break
		}
		if source == nil {
			continue
		}
		event := domain.RunEvent{AgentName: source.AgentName}
		for _, step := range source.RunPath {
			event.RunPath = append(event.RunPath, step.String())
		}
		message, err := consumeOutput(ctx, source, &event, messageID+1, emit)
		if err != nil {
			return nil, err
		}
		if message != nil {
			messageID++
			completed = append(completed, message)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return completed, nil
}

func consumeOutput(ctx context.Context, source *adk.AgentEvent, event *domain.RunEvent, messageID uint64, emit func(domain.RunEvent) error) (*schema.Message, error) {
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
		return nil, source.Err
	}
	if source.Action != nil && source.Action.Interrupted != nil {
		return nil, errors.New("agent: interrupted runs cannot be resumed")
	}
	if output == nil {
		return nil, nil
	}
	event.MessageID, event.Role, event.ToolName = messageID, output.Role, output.ToolName
	if !output.IsStreaming {
		if output.Message == nil {
			return nil, errors.New("agent: missing message output")
		}
		event.Type, event.Message = domain.RunEventMessage, output.Message
		if event.Role == "" {
			event.Role = output.Message.Role
		}
		if event.ToolName == "" {
			event.ToolName = output.Message.ToolName
		}
		return output.Message, emit(*event)
	}
	if output.MessageStream == nil {
		return nil, errors.New("agent: missing message stream")
	}
	var chunks []*schema.Message
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chunk, err := output.MessageStream.Recv()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if chunk == nil {
			continue
		}
		chunks = append(chunks, chunk)
		if event.Role == "" {
			event.Role = chunk.Role
		}
		if event.ToolName == "" {
			event.ToolName = chunk.ToolName
		}
		event.Type, event.Message = domain.RunEventMessageDelta, chunk
		if err := emit(*event); err != nil {
			return nil, err
		}
	}
	if len(chunks) == 0 {
		return nil, errors.New("agent: empty message stream")
	}
	message, err := schema.ConcatMessages(chunks)
	if err != nil {
		return nil, err
	}
	if message.Role == "" {
		message.Role = event.Role
	}
	if message.ToolName == "" {
		message.ToolName = event.ToolName
	}
	event.Type, event.Message = domain.RunEventMessageEnd, nil
	return message, emit(*event)
}
