package devmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cerebrai-app/urban-carnival/internal/app"
)

// fakeStore is an in-memory Store.
type fakeStore struct {
	items   map[string]app.Automation
	created []app.Automation
	nextID  int
}

func newFakeStore() *fakeStore { return &fakeStore{items: map[string]app.Automation{}} }

func (s *fakeStore) GetAutomation(_ context.Context, id string) (app.Automation, error) {
	a, ok := s.items[id]
	if !ok {
		return app.Automation{}, errors.New("automation " + id + " not found")
	}
	return a, nil
}

func (s *fakeStore) CreateAutomation(_ context.Context, d app.AutomationDraft) (app.Automation, error) {
	s.nextID++
	a := app.Automation{
		ID:          fmt.Sprintf("auto-%d", s.nextID),
		Name:        d.Name,
		Description: d.Description,
		Trigger:     d.Trigger,
		Source:      d.Source,
	}
	s.items[a.ID] = a
	s.created = append(s.created, a)
	return a, nil
}

func (s *fakeStore) UpdateAutomation(_ context.Context, a app.Automation) error {
	if _, ok := s.items[a.ID]; !ok {
		return errors.New("automation " + a.ID + " not found")
	}
	s.items[a.ID] = a
	return nil
}

// fakeWriter is a Writer that returns a canned reply and records the task it
// was handed.
type fakeWriter struct {
	reply    string
	err      error
	lastTask string
}

func (w *fakeWriter) Generate(_ context.Context, messages []*schema.Message, _ ...einomodel.Option) (*schema.Message, error) {
	if w.err != nil {
		return nil, w.err
	}
	w.lastTask = messages[len(messages)-1].Content
	return &schema.Message{Role: schema.Assistant, Content: w.reply}, nil
}

// connect wires an in-memory MCP client to a server with the given deps.
func connect(t *testing.T, deps Deps) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "test"}, nil)
	registerTools(server, deps)

	clientT, serverT := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverT, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func callText(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return res
}

func TestCreateAutomationPersistsDisabledDraft(t *testing.T) {
	store := newFakeStore()
	writer := &fakeWriter{reply: "  // authored source\n"}
	cs := connect(t, Deps{Store: store, Writer: writer})

	res := callText(t, cs, toolCreateAutomation, map[string]any{
		"description": "Remind me to stretch every hour",
	})
	if res.IsError {
		t.Fatalf("tool returned an error: %+v", res.Content)
	}
	if len(store.created) != 1 {
		t.Fatalf("created %d automations, want 1", len(store.created))
	}
	got := store.created[0]
	if got.Source != "// authored source" {
		t.Errorf("Source = %q, want the trimmed writer output", got.Source)
	}
	if got.Enabled {
		t.Error("new automation should be a disabled draft")
	}
	if got.Name != "Remind me to stretch every hour" {
		t.Errorf("Name = %q", got.Name)
	}
	if !strings.Contains(writer.lastTask, "Remind me to stretch every hour") {
		t.Errorf("writer task did not include the description: %q", writer.lastTask)
	}
}

func TestEditAutomationRewritesSource(t *testing.T) {
	store := newFakeStore()
	store.items["a1"] = app.Automation{ID: "a1", Name: "Stretch", Description: "hourly stretch", Source: "old"}
	writer := &fakeWriter{reply: "new source"}
	cs := connect(t, Deps{Store: store, Writer: writer})

	res := callText(t, cs, toolEditAutomation, map[string]any{
		"automation_id":    "a1",
		"requested_change": "run every 30 minutes",
	})
	if res.IsError {
		t.Fatalf("tool returned an error: %+v", res.Content)
	}
	if store.items["a1"].Source != "new source" {
		t.Errorf("Source = %q, want %q", store.items["a1"].Source, "new source")
	}
	if !strings.Contains(writer.lastTask, "old") || !strings.Contains(writer.lastTask, "run every 30 minutes") {
		t.Errorf("writer task missing existing source or requested change: %q", writer.lastTask)
	}
}

func TestEditAutomationUnknownID(t *testing.T) {
	cs := connect(t, Deps{Store: newFakeStore(), Writer: &fakeWriter{reply: "x"}})

	res := callText(t, cs, toolEditAutomation, map[string]any{
		"automation_id":    "missing",
		"requested_change": "whatever",
	})
	if !res.IsError {
		t.Fatal("expected an error result for an unknown automation ID")
	}
}

func TestWriterErrorSurfacesAsToolError(t *testing.T) {
	cs := connect(t, Deps{Store: newFakeStore(), Writer: &fakeWriter{err: errors.New("model down")}})

	res := callText(t, cs, toolCreateAutomation, map[string]any{"description": "anything"})
	if !res.IsError {
		t.Fatal("expected an error result when the writer fails")
	}
}

func TestCreateAutomationRequiresDescription(t *testing.T) {
	cs := connect(t, Deps{Store: newFakeStore(), Writer: &fakeWriter{reply: "x"}})

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolCreateAutomation,
		Arguments: map[string]any{},
	})
	// The SDK validates against the inferred schema; a missing required arg
	// comes back as either a call error or an error result.
	if err == nil && !res.IsError {
		t.Fatal("expected an error for a missing description")
	}
}

func TestServerBridgeShape(t *testing.T) {
	srv, err := Start(Deps{Store: newFakeStore(), Writer: &fakeWriter{reply: "x"}})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close(context.Background()) })

	var cfg struct {
		MCPServers map[string]struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(srv.ConfigJSON()), &cfg); err != nil {
		t.Fatalf("ConfigJSON is not valid JSON: %v", err)
	}
	entry, ok := cfg.MCPServers[serverName]
	if !ok {
		t.Fatalf("config has no %q server: %s", serverName, srv.ConfigJSON())
	}
	if entry.Type != "http" || !strings.HasPrefix(entry.URL, "http://127.0.0.1:") || !strings.HasSuffix(entry.URL, mountPath) {
		t.Errorf("server entry = %+v", entry)
	}

	want := []string{"mcp__cerebrai__create_automation", "mcp__cerebrai__edit_automation"}
	got := srv.ToolNames()
	if len(got) != len(want) {
		t.Fatalf("ToolNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ToolNames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
