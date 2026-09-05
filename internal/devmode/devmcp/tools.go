package devmcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/codes"

	"github.com/cerebrai-app/urban-carnival/internal/app"
)

const (
	toolCreateAutomation = "create_automation"
	toolEditAutomation   = "edit_automation"
)

// authoringSystemPrompt frames every automation-writer run. The handler
// persists the reply verbatim as the automation's source, so the model must
// return code and nothing else.
const authoringSystemPrompt = `You are cerebrai's automation writer. Given a task, produce the source code for a single automation that carries it out.
Return ONLY the automation's source code: no explanation, no commentary, no Markdown code fences.`

// createAutomationInput is the create_automation argument schema. Both the
// go-sdk's schema inference and validation treat non-omitempty fields as
// required, and read the "jsonschema" tag as the property description.
type createAutomationInput struct {
	Description string `json:"description" jsonschema:"natural-language description of the automation to create: what triggers it and what it should do"`
}

// editAutomationInput is the edit_automation argument schema.
type editAutomationInput struct {
	AutomationID    string `json:"automation_id" jsonschema:"ID of the existing automation to modify"`
	RequestedChange string `json:"requested_change" jsonschema:"natural-language description of the change to make to that automation"`
}

// registerTools binds create_automation and edit_automation on s. Each
// handler runs the automation writer to author source, then persists the
// result as a disabled draft for the user to review (DESIGN.md §5.5).
func registerTools(s *mcp.Server, deps Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        toolCreateAutomation,
		Description: "Author a new automation from a natural-language description and save it as a disabled draft for the user to review.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in createAutomationInput) (*mcp.CallToolResult, any, error) {
		source, err := runWriter(ctx, deps.Writer, "Write a new automation.\n\nDescription:\n"+in.Description)
		if err != nil {
			return nil, nil, err
		}
		created, err := deps.Store.CreateAutomation(ctx, app.AutomationDraft{
			Name:        deriveName(in.Description),
			Description: in.Description,
			Trigger:     "manual",
			Source:      source,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("save automation: %w", err)
		}
		return textResult("Created automation %q (id %s) as a disabled draft. Review it in the Automations tab before enabling.", created.Name, created.ID), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        toolEditAutomation,
		Description: "Revise an existing automation given its ID and a requested change, saving the updated draft.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in editAutomationInput) (*mcp.CallToolResult, any, error) {
		existing, err := deps.Store.GetAutomation(ctx, in.AutomationID)
		if err != nil {
			return nil, nil, err
		}
		task := fmt.Sprintf(
			"Revise an existing automation.\n\nCurrent description:\n%s\n\nCurrent source:\n%s\n\nRequested change:\n%s",
			existing.Description, existing.Source, in.RequestedChange,
		)
		source, err := runWriter(ctx, deps.Writer, task)
		if err != nil {
			return nil, nil, err
		}
		existing.Source = source
		if err := deps.Store.UpdateAutomation(ctx, existing); err != nil {
			return nil, nil, fmt.Errorf("save automation: %w", err)
		}
		return textResult("Updated automation %q (id %s). Review the changes in the Automations tab.", existing.Name, existing.ID), nil, nil
	})
}

// runWriter asks the automation writer to author source for task and returns
// the trimmed reply text.
func runWriter(ctx context.Context, w Writer, task string) (string, error) {
	ctx, span := tracer.Start(ctx, "automation-writer generate")
	defer span.End()

	reply, err := w.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: authoringSystemPrompt},
		{Role: schema.User, Content: task},
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("automation writer: %w", err)
	}
	if reply == nil {
		return "", errors.New("automation writer: no reply produced")
	}
	return strings.TrimSpace(reply.Content), nil
}

func textResult(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
	}
}

// automationNameMaxRunes caps a derived automation name.
const automationNameMaxRunes = 60

// deriveName turns a description into a short automation name: its first line,
// trimmed and truncated (see app.Summarize), or a placeholder when empty.
func deriveName(description string) string {
	if name := app.Summarize(description, automationNameMaxRunes); name != "" {
		return name
	}
	return "Untitled automation"
}
