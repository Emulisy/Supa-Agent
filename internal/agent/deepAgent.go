package agent

import (
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)


type Config = TypedConfig[*schema.Message]

type TypedConfig[M adk.MessageType] struct {
    Name        string              // Agent identifier name
    Description string              // Purpose description
    ChatModel   model.BaseModel[M]  // Required; must support model.WithTools
    Instruction string              // System prompt; uses built-in default Prompt when empty

    // Sub-Agents (bound to TaskTool)
    SubAgents []adk.TypedAgent[M]

    // Custom tools
    ToolsConfig  adk.ToolsConfig
    MaxIteration int // Maximum reasoning iteration count

    // File system (choose one or combine)
    Backend        filesystem.Backend        // Registers ls/read_file/write_file/edit_file/glob/grep
    Shell          filesystem.Shell          // Registers execute (mutually exclusive with StreamingShell)
    StreamingShell filesystem.StreamingShell  // Registers execute (streaming, mutually exclusive with Shell)

    // Built-in feature toggles
    WithoutWriteTodos      bool // true disables the write_todos tool
    WithoutGeneralSubAgent bool // true disables the default general-purpose sub-Agent

    // TaskTool description generator (customize the task tool's description)
    TaskToolDescriptionGenerator func(ctx context.Context, agents []adk.TypedAgent[M]) (string, error)

    // Extensions
    Middlewares []adk.AgentMiddleware                   // struct-based middleware
    Handlers    []adk.TypedChatModelAgentMiddleware[M]  // interface-based handlers

    // Model fault tolerance
    ModelRetryConfig    *adk.TypedModelRetryConfig[M]
    ModelFailoverConfig *adk.ModelFailoverConfig[M]

    // Output storage (written to session via AddSessionValue)
    OutputKey string
}
