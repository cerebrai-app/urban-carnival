package automationagent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
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

func TestRespondOpensLoopSpan(t *testing.T) {
	sr := recordSpans(t)

	loop, err := New(context.Background(), echoProvider{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := loop.Respond(context.Background(), []*schema.Message{{Role: schema.User, Content: "hi"}}); err != nil {
		t.Fatalf("Respond: %v", err)
	}

	span := findSpan(sr, "automation-agent loop")
	if span == nil {
		t.Fatalf("no loop span recorded; got %v", spanNames(sr))
	}
	if !hasIntAttr(span, "automationagent.seed_messages", 1) {
		t.Errorf("span missing seed_messages=1: %v", span.Attributes())
	}
	if !hasIntAttr(span, "spawn_agent.depth", 0) {
		t.Errorf("span missing spawn_agent.depth=0: %v", span.Attributes())
	}
}

func TestRespondStartsRootTraceLinkedToCaller(t *testing.T) {
	sr := recordSpans(t)

	// An ambient span standing in for the chat turn that hands off to the loop.
	ctx, parent := tracer.Start(context.Background(), "caller")

	loop, err := New(ctx, echoProvider{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := loop.Respond(ctx, []*schema.Message{{Role: schema.User, Content: "hi"}}); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	parent.End()

	span := findSpan(sr, "automation-agent loop")
	if span == nil {
		t.Fatalf("no loop span recorded; got %v", spanNames(sr))
	}
	if span.Parent().IsValid() {
		t.Errorf("loop span has a parent %v, want a new root", span.Parent().SpanID())
	}
	if span.SpanContext().TraceID() == parent.SpanContext().TraceID() {
		t.Errorf("loop span shares the caller's trace ID; want a fresh trace")
	}
	assertFollowsFromLink(t, span, parent.SpanContext())
}

func TestRespondNestsSpawnedLoopViaLink(t *testing.T) {
	sr := recordSpans(t)

	loop, err := New(context.Background(), spawningProvider{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := loop.Respond(context.Background(), []*schema.Message{{Role: schema.User, Content: "hello"}}); err != nil {
		t.Fatalf("Respond: %v", err)
	}

	var outer, inner sdktrace.ReadOnlySpan
	for _, s := range sr.Ended() {
		if s.Name() != "automation-agent loop" {
			continue
		}
		if hasIntAttr(s, "spawn_agent.depth", 1) {
			inner = s
		} else {
			outer = s
		}
	}
	if outer == nil || inner == nil {
		t.Fatalf("want an outer and a spawned loop span; got %v", spanNames(sr))
	}
	if inner.SpanContext().TraceID() == outer.SpanContext().TraceID() {
		t.Errorf("spawned loop span shares the calling loop's trace; want a fresh trace")
	}
	assertFollowsFromLink(t, inner, outer.SpanContext())
}

func assertFollowsFromLink(t *testing.T, s sdktrace.ReadOnlySpan, want trace.SpanContext) {
	t.Helper()
	for _, l := range s.Links() {
		if l.SpanContext.SpanID() != want.SpanID() {
			continue
		}
		for _, a := range l.Attributes {
			if a.Key == semconv.OpenTracingRefTypeKey && a.Value.AsString() == "follows_from" {
				return
			}
		}
		t.Errorf("link to %v is missing the follows_from ref type: %v", want.SpanID(), l.Attributes)
		return
	}
	t.Errorf("span %q has no link back to %v; links = %v", s.Name(), want.SpanID(), s.Links())
}

func TestRespondMarksErrorSpan(t *testing.T) {
	sr := recordSpans(t)

	loop, err := New(context.Background(), Unconfigured{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := loop.Respond(context.Background(), []*schema.Message{{Role: schema.User, Content: "hi"}}); err == nil {
		t.Fatal("expected an error from an unconfigured provider")
	}

	span := findSpan(sr, "automation-agent loop")
	if span == nil {
		t.Fatalf("no loop span recorded")
	}
	if span.Status().Code != codes.Error {
		t.Errorf("span status = %+v, want Error", span.Status())
	}
}

func findSpan(sr *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	for _, s := range sr.Ended() {
		if s.Name() == name {
			return s
		}
	}
	return nil
}

func spanNames(sr *tracetest.SpanRecorder) []string {
	var names []string
	for _, s := range sr.Ended() {
		names = append(names, s.Name())
	}
	return names
}

func hasIntAttr(s sdktrace.ReadOnlySpan, key string, val int64) bool {
	for _, a := range s.Attributes() {
		if string(a.Key) == key && a.Value.AsInt64() == val {
			return true
		}
	}
	return false
}
