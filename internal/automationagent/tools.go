package automationagent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// maxSpawnDepth caps how deeply spawn_agent may nest. Each spawned Loop is
// built like any other, so it gets its own spawn_agent tool and its own step
// budget (maxAgentSteps) — without this, a chain of loops could nest without
// bound. The current depth travels on the context so every sub-loop sees its
// own level.
const maxSpawnDepth = 3

// spawnDepthKey is the context key carrying the current spawn_agent nesting
// depth (0 at the top-level Loop).
type spawnDepthKey struct{}

func spawnDepth(ctx context.Context) int {
	d, _ := ctx.Value(spawnDepthKey{}).(int)
	return d
}

func withSpawnDepth(ctx context.Context, d int) context.Context {
	return context.WithValue(ctx, spawnDepthKey{}, d)
}

// spawnAgentInput is the argument schema the "spawn_agent" tool exposes to
// the model: a single, self-contained task for the new loop to work on.
type spawnAgentInput struct {
	Task string `json:"task" jsonschema:"required" jsonschema_description:"A self-contained task or question for the new agent loop to work on independently"`
}

// newSpawnAgentTool returns the "spawn_agent" tool: it lets a running Loop
// create a brand new, independent Loop over the same provider, run it to
// completion on a single task, and get its final reply back as the tool
// result. This gives the assistant a way to delegate a self-contained
// sub-task to a fresh conversation instead of solving it inline and growing
// the calling loop's own history (DESIGN.md §5, §9.3).
//
// Recursion note: the spawned Loop is built the same way as any other, so it
// gets its own spawn_agent tool and may spawn further sub-loops. Nesting is
// capped at maxSpawnDepth (tracked on the context); within one loop the model
// is additionally bounded by react.AgentConfig.MaxStep (maxAgentSteps).
func newSpawnAgentTool(provider ModelProvider) (tool.InvokableTool, error) {
	return utils.InferTool(
		"spawn_agent",
		"Create a new, independent agent loop to work on a self-contained task and return its final reply. "+
			"Use this to delegate a sub-task rather than solving it inline.",
		func(ctx context.Context, in spawnAgentInput) (string, error) {
			depth := spawnDepth(ctx) + 1
			if depth > maxSpawnDepth {
				return "", fmt.Errorf("spawn_agent: recursion depth limit (%d) reached", maxSpawnDepth)
			}
			ctx = withSpawnDepth(ctx, depth)

			sub, err := New(ctx, provider)
			if err != nil {
				return "", fmt.Errorf("spawn_agent: create loop: %w", err)
			}
			reply, err := sub.Respond(ctx, []*schema.Message{{Role: schema.User, Content: in.Task}})
			if err != nil {
				return "", fmt.Errorf("spawn_agent: %w", err)
			}
			return reply.Content, nil
		},
	)
}

// defaultTools returns the tools every Loop built by New exposes to its
// model, bound to provider so a tool like spawn_agent can build further
// Loops of its own.
func defaultTools(provider ModelProvider) ([]tool.BaseTool, error) {
	spawnAgent, err := newSpawnAgentTool(provider)
	if err != nil {
		return nil, fmt.Errorf("spawn_agent tool: %w", err)
	}
	return []tool.BaseTool{spawnAgent}, nil
}
