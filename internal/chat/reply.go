package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/schema"

	"github.com/cerebrai-app/urban-carnival/internal/app"
)

// Reply runs one chat turn (DESIGN.md §5.2): it converts the persisted
// conversation history into schema messages and asks provider for a single
// reply, returning its text plus the provider's conversation handle to
// persist and replay on the next turn (priorHandle is the one from last time,
// empty on the first turn). No tool-execution loop, and — for now — no
// create_automation/edit_automation intent bind: the only provider that
// exists (devmode/claudecode) drives its own tool use via the in-process MCP
// server (internal/devmode/devmcp), so the chat→writer handoff happens inside
// that MCP handler, not here. A native tool-calling provider will need the
// bind plus a ToolCalls branch added here.
func Reply(ctx context.Context, provider ConversationProvider, history []app.Message, priorHandle string) (reply, handle string, err error) {
	msg, handle, err := provider.Reply(ctx, priorHandle, toSchemaMessages(history))
	if err != nil {
		return "", "", fmt.Errorf("chat reply: %w", err)
	}
	if msg == nil {
		return "", "", errors.New("chat reply: provider returned no message")
	}
	return msg.Content, handle, nil
}

// toSchemaMessages converts persisted history into schema messages. The
// persisted model has no system role, so this only distinguishes "assistant"
// from everything-else-as-user (DESIGN.md §5.2).
func toSchemaMessages(history []app.Message) []*schema.Message {
	msgs := make([]*schema.Message, 0, len(history))
	for _, m := range history {
		role := schema.User
		if m.Role == "assistant" {
			role = schema.Assistant
		}
		msgs = append(msgs, &schema.Message{Role: role, Content: m.Content})
	}
	return msgs
}
