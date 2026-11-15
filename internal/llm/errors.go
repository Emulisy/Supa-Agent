package llm

import "errors"

var (
	// ErrUnsupportedModel means a model selector is not present in the gateway config.
	ErrUnsupportedModel = errors.New("llm: unsupported model")
	// ErrGenerateFailed means the selected model failed to generate a response.
	ErrGenerateFailed = errors.New("llm: failed to generate response")
	// ErrStreamFailed means the selected model failed to start a response stream.
	ErrStreamFailed = errors.New("llm: failed to start response stream")
	// ErrBindToolsFailed means the selected model failed to bind tools.
	ErrBindToolsFailed = errors.New("llm: failed to bind tools")
)
