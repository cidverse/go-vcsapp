package vcsapp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRender(t *testing.T) {
	output, err := Render("Test Content: {{ .Content }}", map[string]interface{}{
		"Content": "Example",
	})
	expected := "Test Content: Example"
	assert.NoError(t, err)
	assert.Equal(t, expected, string(output))
}

type DummyStatus string

const (
	StatusActive DummyStatus = "active"
)

func TestRenderContains(t *testing.T) {
	data := map[string]interface{}{
		"statusList": []DummyStatus{
			StatusActive,
		},
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "Enum in slice found",
			template: `{{ if contains .statusList "active" }}TRUE{{ else }}FALSE{{ end }}`,
			expected: "TRUE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := Render(tt.template, data)

			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}

			if string(output) != tt.expected {
				t.Errorf("%s: expected %q, got %q", tt.name, tt.expected, string(output))
			}
		})
	}
}
