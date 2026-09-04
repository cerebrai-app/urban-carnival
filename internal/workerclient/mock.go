package workerclient

import (
	"context"
	"fmt"
	"time"
)

// Mock is an in-memory Client for running and developing the desktop UI
// before the background worker's local API exists.
type Mock struct {
	automations []Automation
}

// NewMock returns a Mock seeded with a couple of example automations.
func NewMock() *Mock {
	now := time.Now()
	return &Mock{
		automations: []Automation{
			{
				ID:          "water-plants",
				Name:        "Water the plants",
				Description: "Remind me to water plants every Tuesday morning.",
				Trigger:     "schedule: 0 9 * * 2",
				Enabled:     true,
				UpdatedAt:   now,
			},
			{
				ID:          "inbox-summary",
				Name:        "Inbox summary",
				Description: "Summarize my inbox every morning.",
				Trigger:     "schedule: 0 8 * * *",
				Enabled:     false,
				UpdatedAt:   now,
			},
		},
	}
}

// SendMessage echoes a placeholder assistant reply.
func (m *Mock) SendMessage(_ context.Context, content string) (Message, error) {
	return Message{
		ID:        "mock",
		Role:      "assistant",
		Content:   fmt.Sprintf("(mock worker) you said: %q", content),
		CreatedAt: time.Now(),
	}, nil
}

// ListAutomations returns the seeded automations.
func (m *Mock) ListAutomations(_ context.Context) ([]Automation, error) {
	out := make([]Automation, len(m.automations))
	copy(out, m.automations)
	return out, nil
}

// SetAutomationEnabled updates the enabled flag on the matching automation.
func (m *Mock) SetAutomationEnabled(_ context.Context, id string, enabled bool) error {
	for i := range m.automations {
		if m.automations[i].ID == id {
			m.automations[i].Enabled = enabled
			m.automations[i].UpdatedAt = time.Now()
			return nil
		}
	}
	return fmt.Errorf("automation %q not found", id)
}
