// Package model defines cerebrai's pluggable chat-model abstraction. The
// agent loop (internal/agent) talks to whatever Provider is configured
// here rather than to any specific vendor's SDK, so Anthropic, OpenAI, and
// local models can all sit behind the same interface (DESIGN.md §5).
package model

import (
	"context"
	"errors"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Provider is a chat model the agent loop can invoke. It's Eino's
// ToolCallingChatModel interface, aliased here so the rest of the codebase
// depends on this package rather than importing Eino directly — vendor
// wiring (Anthropic, OpenAI, local) stays confined to whatever package
// constructs a concrete Provider.
type Provider = einomodel.ToolCallingChatModel

// ErrNotConfigured is returned by every Unconfigured call.
var ErrNotConfigured = errors.New("model: no provider configured")

// Unconfigured is a placeholder Provider used until a real model provider
// is wired in, so the agent loop can be built and tested before any vendor
// integration exists.
type Unconfigured struct{}

func (Unconfigured) Generate(context.Context, []*schema.Message, ...einomodel.Option) (*schema.Message, error) {
	return nil, ErrNotConfigured
}

func (Unconfigured) Stream(context.Context, []*schema.Message, ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, ErrNotConfigured
}

func (u Unconfigured) WithTools([]*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return u, nil
}
