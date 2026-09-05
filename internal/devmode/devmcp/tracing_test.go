package devmcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// recordSpans installs a fresh in-memory tracer provider as the global one for
// the duration of the test and returns its recorder.
func recordSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr)))
	tracer = otel.Tracer("test")
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return sr
}

func TestTracingMiddlewareNamesToolCallSpans(t *testing.T) {
	sr := recordSpans(t)

	store := newFakeStore()
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "test"}, nil)
	server.AddReceivingMiddleware(tracingMiddleware)
	registerTools(server, Deps{Store: store, Writer: &fakeWriter{reply: "src"}})

	clientT, serverT := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), serverT, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "test"}, nil)
	cs, err := client.Connect(context.Background(), clientT, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolCreateAutomation,
		Arguments: map[string]any{"description": "stretch hourly"},
	}); err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	var toolSpan sdktrace.ReadOnlySpan
	for _, s := range sr.Ended() {
		if s.Name() == "mcp tools/call "+toolCreateAutomation {
			toolSpan = s
		}
	}
	if toolSpan == nil {
		t.Fatalf("no tools/call span recorded; got %v", spanNames(sr))
	}
	if !hasAttr(toolSpan, "mcp.tool.name", toolCreateAutomation) {
		t.Errorf("tool span missing mcp.tool.name attribute: %v", toolSpan.Attributes())
	}
	// The automation-writer child span should sit under the tool span.
	for _, s := range sr.Ended() {
		if s.Name() == "automation-writer generate" &&
			s.Parent().SpanID() != toolSpan.SpanContext().SpanID() {
			t.Errorf("automation-writer span is not a child of the tool span")
		}
	}
}

func TestTracingMiddlewareMarksErrorResults(t *testing.T) {
	sr := recordSpans(t)

	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "test"}, nil)
	server.AddReceivingMiddleware(tracingMiddleware)
	registerTools(server, Deps{Store: newFakeStore(), Writer: &fakeWriter{reply: "x"}})

	clientT, serverT := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), serverT, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "test"}, nil)
	cs, err := client.Connect(context.Background(), clientT, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	// Editing an unknown automation yields an IsError tool result.
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolEditAutomation,
		Arguments: map[string]any{"automation_id": "missing", "requested_change": "x"},
	}); err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	for _, s := range sr.Ended() {
		if s.Name() == "mcp tools/call "+toolEditAutomation {
			if s.Status().Code != codes.Error {
				t.Errorf("error tool result did not set span status to Error: %+v", s.Status())
			}
			return
		}
	}
	t.Fatalf("no edit_automation span recorded; got %v", spanNames(sr))
}

func spanNames(sr *tracetest.SpanRecorder) []string {
	var names []string
	for _, s := range sr.Ended() {
		names = append(names, s.Name())
	}
	return names
}

func hasAttr(s sdktrace.ReadOnlySpan, key, val string) bool {
	for _, a := range s.Attributes() {
		if string(a.Key) == key && a.Value.AsString() == val {
			return true
		}
	}
	return false
}
