// Package automationagent builds the automation writer agent loop (DESIGN.md
// §5.3): the genuinely agentic loop that authors or edits an automation,
// invoked by the chat handoff (§5.2) rather than wrapped around every chat
// turn. It sits between the seed task it's handed and a pluggable
// ModelProvider (Provider resolves the process-global one). Orchestration is
// built on Eino's ReAct agent, so the loop can call tools (see tools.go) as
// part of producing its result. Each Respond call opens its own root
// OpenTelemetry trace (see tracer), linked back to whatever invoked it,
// exported through the global provider internal/telemetry configures.
package automationagent

import (
	"context"
	"errors"
	"fmt"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/cerebrai-app/urban-carnival/internal/devmode"
)

// tracer emits spans for this package under the global provider configured by
// internal/telemetry.
var tracer = otel.Tracer("github.com/cerebrai-app/urban-carnival/internal/automationagent")

// maxAgentSteps bounds the tool-calling rounds in one Loop (Eino's react
// MaxStep). Set explicitly rather than relying on the package default (12) so
// a runaway model can't spin indefinitely and the budget lives in one place.
// spawn_agent recursion is bounded separately (see maxSpawnDepth), since each
// spawned Loop gets its own budget.
const maxAgentSteps = 20

// ModelProvider is the model the automation writer's loop invokes (DESIGN.md
// §5.3), potentially many times per task as it calls tools. It's Eino's
// ToolCallingChatModel, a named interface rather than an alias so it can
// diverge from chat.ModelProvider — the automation writer can run on a
// different (e.g. stronger, code-focused) model than chat.
type ModelProvider interface {
	einomodel.ToolCallingChatModel
}

// ErrNotConfigured is returned by every Unconfigured call.
var ErrNotConfigured = errors.New("automationagent: no model provider configured")

// Unconfigured is a placeholder ModelProvider used until the process-global
// automation writer model is wired in, so the loop can be built and tested
// before any vendor integration exists.
type Unconfigured struct{}

var _ ModelProvider = Unconfigured{}

func (Unconfigured) Generate(context.Context, []*schema.Message, ...einomodel.Option) (*schema.Message, error) {
	return nil, ErrNotConfigured
}

func (Unconfigured) Stream(context.Context, []*schema.Message, ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, ErrNotConfigured
}

func (u Unconfigured) WithTools([]*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return u, nil
}

// Provider resolves the process-global automation writer model (DESIGN.md
// §5.3) to a ModelProvider. Unlike chat's model this is not per-session:
// production picks it once during user setup, developer builds hard-code it
// (devmode.AgentModel). Falls back to Unconfigured until that setup exists /
// in production builds.
func Provider() ModelProvider {
	if p := devmode.Provider(devmode.AgentModel()); p != nil {
		return p
	}
	return Unconfigured{}
}

// Loop runs one automation-writing task to completion: given the seed
// message(s), it invokes the underlying ModelProvider — calling any tools it
// decides to use along the way — and returns its final reply.
type Loop struct {
	agent *react.Agent
}

// New compiles a Loop around provider, wired with the tools every Loop
// exposes to its model (see defaultTools). This is the seam DESIGN.md §5.6
// calls for: swap provider for a real Anthropic/OpenAI/local ModelProvider
// without touching anything downstream of Loop.
//
// Not every provider supports tools — e.g. claudecode.ChatModel.WithTools
// rejects a non-empty tool list outright, since the Claude Code CLI drives
// its own tool use internally. New surfaces that as an error here rather
// than silently building a tool-less loop.
func New(ctx context.Context, provider ModelProvider) (*Loop, error) {
	tools, err := defaultTools(provider)
	if err != nil {
		return nil, fmt.Errorf("build agent loop tools: %w", err)
	}

	a, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: provider,
		ToolsConfig:      compose.ToolsNodeConfig{Tools: tools},
		MaxStep:          maxAgentSteps,
	})
	if err != nil {
		return nil, fmt.Errorf("compile agent loop: %w", err)
	}
	return &Loop{agent: a}, nil
}

// Respond runs the seed messages through the loop — including any tool calls
// the model makes along the way — and returns its final reply message.
//
// Each call opens its own root trace so one loop's work — including any tools
// it drives — is a self-contained trace rather than a deep branch of whatever
// invoked it. Whatever span was active in ctx (the chat turn that handed off,
// or the calling loop for a spawn_agent delegation) is preserved as a
// follows_from link, so the traces stay navigable from one another.
// spawn_agent.depth records how deep a spawn_agent delegation chain runs.
func (l *Loop) Respond(ctx context.Context, history []*schema.Message) (*schema.Message, error) {
	opts := []trace.SpanStartOption{
		trace.WithNewRoot(),
		trace.WithAttributes(
			attribute.Int("automationagent.seed_messages", len(history)),
			attribute.Int("automationagent.max_steps", maxAgentSteps),
			attribute.Int("spawn_agent.depth", spawnDepth(ctx)),
		),
	}
	if trace.SpanContextFromContext(ctx).IsValid() {
		opts = append(opts, trace.WithLinks(trace.LinkFromContext(ctx, semconv.OpenTracingRefTypeFollowsFrom)))
	}

	ctx, span := tracer.Start(ctx, "automation-agent loop", opts...)
	defer span.End()

	reply, err := l.agent.Generate(ctx, history)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("agent loop: %w", err)
	}
	if reply == nil {
		err := errors.New("agent loop: no reply produced")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return reply, nil
}
