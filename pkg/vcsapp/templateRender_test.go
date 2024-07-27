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
