# LLM gateway — Phase 1

The gateway maps stable logical model names to reusable Eino clients. DeepSeek
is the only provider registered by the chat command. Each logical model must
reference exactly one deployment; multiple names may share the same client.

`gateway.LoadConfigFile` reads a single YAML document, rejects unknown fields,
and resolves each deployment's `api_key_env` from the process environment. The
chat command loads `.env` first, with existing shell variables taking precedence.
Keep credentials in the environment and physical model IDs in `configs/llm.yaml`.
Go callers can also supply `DeploymentConfig.APIKey` directly.

```go
providers := provider.NewRegistry()
if err := providers.Register(provider.DeepSeek, provider.NewDeepSeek); err != nil {
    return err
}
cfg, err := gateway.LoadConfigFile("configs/llm.yaml")
if err != nil {
    return err
}
gw, err := gateway.New(ctx, cfg, providers)
if err != nil {
    return err
}
chatModel, err := gw.Model("agent-default")
if err != nil {
    return err
}
response, err := chatModel.Generate(ctx, messages)
```

Import `github.com/Emulisy/Supa-Agent/internal/llm/gateway` and
`github.com/Emulisy/Supa-Agent/internal/llm/provider` from within this module.
`gw.Model("")` selects `default_model`. The returned model implements
`model.ToolCallingChatModel` and can be passed directly to Eino ADK's
`ChatModelAgentConfig.Model`.

`Generate` and `Stream` forward all Eino options, including provider-specific
options. A stream's caller must close its reader. `WithTools` returns a new
wrapper around a separate tool-bound client without mutating the base model;
per-request `model.WithTools` options also pass through to DeepSeek. Empty tool
bindings follow the native adapter's validation and return an error.

The gateway validates the configuration and checks every provider registration
before constructing clients. Deployment and provider registries are populated
at startup and remain read-only afterward. Model lookup uses only logical names.

From the repository root, after setting `DEEPSEEK_API_KEY` in your shell or `.env`:

```powershell
go run ./cmd/chat "Explain what an AI agent is."
go run ./cmd/chat -model agent-pro "Explain tool calling."
go run ./cmd/chat -model agent-fast -stream -timeout 60s "Hello"
```

Routing among deployments, retry/fallback policies, traffic controls and dynamic
configuration belong to later phases.
