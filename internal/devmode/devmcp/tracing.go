package devmcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// tracer emits spans for this package under the global provider configured by
// internal/telemetry.
var tracer = otel.Tracer("github.com/cerebrai-app/urban-carnival/internal/devmode/devmcp")

// tracingMiddleware wraps every incoming MCP method call in an OpenTelemetry
// server span. A tools/call span is named "mcp tools/call <tool>" and carries
// the tool name; other methods (initialize, tools/list, …) get "mcp <method>".
// Trace context on the originating HTTP request is extracted first, so a tool
// call joins the caller's trace when one is propagated. A transport error, or
// a tool result flagged IsError, marks the span as failed.
func tracingMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if extra := req.GetExtra(); extra != nil && extra.Header != nil {
			ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(extra.Header))
		}

		spanName := "mcp " + method
		attrs := []attribute.KeyValue{attribute.String("mcp.method", method)}
		if ctr, ok := req.(*mcp.CallToolRequest); ok && ctr.Params != nil {
			spanName = "mcp tools/call " + ctr.Params.Name
			attrs = append(attrs, attribute.String("mcp.tool.name", ctr.Params.Name))
		}
		if sess := req.GetSession(); sess != nil {
			if id := sess.ID(); id != "" {
				attrs = append(attrs, attribute.String("mcp.session.id", id))
			}
		}

		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attrs...),
		)
		defer span.End()

		result, err := next(ctx, method, req)
		switch {
		case err != nil:
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		case isErrorResult(result):
			span.SetStatus(codes.Error, "tool call returned an error result")
		}
		return result, err
	}
}

// isErrorResult reports whether result is a tool call that came back flagged as
// an error (a handler returning a non-nil error, which the SDK converts into an
// IsError result rather than a transport failure).
func isErrorResult(result mcp.Result) bool {
	ctr, ok := result.(*mcp.CallToolResult)
	return ok && ctr.IsError
}
