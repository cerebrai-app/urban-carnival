// Package devmcp is cerebrai's in-process MCP server, run only in developer
// builds (DESIGN.md §5.6). It serves cerebrai's own tools —
// create_automation and edit_automation — to the local Claude Code CLI
// subprocess that backs the dev chat/automation-writer model
// (internal/devmode/claudecode).
//
// The CLI executes MCP tool calls itself, inside a single RunPromptCtx
// invocation, rather than surfacing them to the caller. So for the dev
// provider the automation writer is invoked *here*, inside the tool handler,
// synchronously before the chat turn's Generate returns (DESIGN.md §5.2
// provider caveat). A native tool-calling provider (hosted Anthropic/OpenAI)
// would not need this bridge.
//
// Transport is streamable HTTP on a random 127.0.0.1 port; ConfigJSON
// produces the inline --mcp-config document and ToolNames the qualified tool
// names that internal/devmode.ChatProvider hands to claudecode.WithMCP.
package devmcp
