package model

import (
	"context"
	"errors"
	"testing"
)

func TestUnconfigured(t *testing.T) {
	ctx := context.Background()
	var p Provider = Unconfigured{}

	if _, err := p.Generate(ctx, nil); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Generate error = %v, want ErrNotConfigured", err)
	}

	if _, err := p.Stream(ctx, nil); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Stream error = %v, want ErrNotConfigured", err)
	}

	withTools, err := p.WithTools(nil)
	if err != nil {
		t.Fatalf("WithTools: %v", err)
	}
	if _, err := withTools.Generate(ctx, nil); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("WithTools(...).Generate error = %v, want ErrNotConfigured", err)
	}
}
