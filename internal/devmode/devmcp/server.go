package devmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cerebrai-app/urban-carnival/internal/app"
	"github.com/cerebrai-app/urban-carnival/internal/config"
)

// serverName is the MCP server name the CLI qualifies tool calls with:
// mcp__cerebrai__<tool>.
const serverName = "cerebrai"

// mountPath is where the streamable HTTP handler is mounted.
const mountPath = "/mcp"

// Store is the automation persistence the tool handlers need (a subset of
// storage.SQLite, satisfied structurally).
type Store interface {
	GetAutomation(ctx context.Context, id string) (app.Automation, error)
	CreateAutomation(ctx context.Context, d app.AutomationDraft) (app.Automation, error)
	UpdateAutomation(ctx context.Context, a app.Automation) error
}

// Writer authors automation source from a seed task. It's the automation
// writer model (automationagent.Provider()'s return) — for the dev
// claudecode provider a single Generate is itself an agentic run, since the
// CLI drives its own tool use.
type Writer interface {
	Generate(ctx context.Context, messages []*schema.Message, opts ...einomodel.Option) (*schema.Message, error)
}

// Deps is what Start needs wired in from app setup.
type Deps struct {
	Store  Store
	Writer Writer
}

// Server is a running in-process MCP server. Close it at shutdown.
type Server struct {
	http *http.Server
	url  string
}

// Start builds the MCP server, registers the automation tools, and serves it
// over streamable HTTP on a random loopback port. Close the returned Server at
// shutdown.
func Start(deps Deps) (*Server, error) {
	if deps.Store == nil || deps.Writer == nil {
		return nil, errors.New("devmcp: Store and Writer are required")
	}

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: config.Version}, nil)
	mcpServer.AddReceivingMiddleware(tracingMiddleware)
	registerTools(mcpServer, deps)

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpServer }, nil)
	mux := http.NewServeMux()
	mux.Handle(mountPath, handler)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("devmcp: listen: %w", err)
	}

	httpSrv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("devmcp: server stopped", "error", err)
		}
	}()

	return &Server{
		http: httpSrv,
		url:  fmt.Sprintf("http://%s%s", ln.Addr().String(), mountPath),
	}, nil
}

// URL is the server's endpoint, for logging.
func (s *Server) URL() string { return s.url }

// ConfigJSON is the inline `--mcp-config` document pointing the Claude Code
// CLI at this server (satisfies devmode.MCPBridge).
func (s *Server) ConfigJSON() string {
	type entry struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	doc := struct {
		MCPServers map[string]entry `json:"mcpServers"`
	}{MCPServers: map[string]entry{serverName: {Type: "http", URL: s.url}}}

	b, err := json.Marshal(doc)
	if err != nil {
		// The document is a fixed shape over a plain string; marshaling it
		// cannot fail in practice.
		panic(fmt.Sprintf("devmcp: marshal config: %v", err))
	}
	return string(b)
}

// ToolNames is the qualified names of the tools this server serves, for the
// CLI's --allowedTools (satisfies devmode.MCPBridge).
func (s *Server) ToolNames() []string {
	return []string{
		"mcp__" + serverName + "__" + toolCreateAutomation,
		"mcp__" + serverName + "__" + toolEditAutomation,
	}
}

// Close stops serving. Safe to call once.
func (s *Server) Close(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
