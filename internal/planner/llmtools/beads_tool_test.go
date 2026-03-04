package llmtools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBeadsTool(t *testing.T) {
	bt := NewBeadsTool(".")

	t.Run("Unsupported operation", func(t *testing.T) {
		resp, err := bt.Run(nil, BeadsArgs{Op: "invalid"})
		assert.NoError(t, err)
		assert.Contains(t, resp.Error, "unsupported operation")
	})

	t.Run("List operation", func(t *testing.T) {
		// Just check it doesn't fail basic execution (requires bd in path)
		resp, err := bt.Run(nil, BeadsArgs{Op: "list"})
		assert.NoError(t, err)
		// We don't assert ExitCode 0 or non-empty Stdout because bd might not be initialized
		// or there might be no issues in the current repo.
		// The tool itself should return a valid response even if bd fails.
		assert.NotNil(t, resp)
	})
}

func TestBeadsToolReason(t *testing.T) {
	bt := NewBeadsTool(".")

	ops := []string{"close", "reopen", "delete"}
	for _, op := range ops {
		t.Run(op+" without reason", func(t *testing.T) {
			resp, err := bt.Run(nil, BeadsArgs{Op: op, Args: []string{"some-id"}})
			assert.NoError(t, err)
			assert.Contains(t, resp.Error, "requires a non-empty --reason")
		})

		t.Run(op+" with reason", func(t *testing.T) {
			// This will attempt to exec bd, so we check if it passed validation
			resp, err := bt.Run(nil, BeadsArgs{Op: op, Args: []string{"some-id", "--reason", "test"}})
			assert.NoError(t, err)
			// It might fail because 'some-id' doesn't exist, but it should NOT be a validation error
			assert.NotContains(t, resp.Error, "requires a non-empty --reason")
		})

		t.Run(op+" with reason= format", func(t *testing.T) {
			resp, err := bt.Run(nil, BeadsArgs{Op: op, Args: []string{"some-id", "--reason=test"}})
			assert.NoError(t, err)
			assert.NotContains(t, resp.Error, "requires a non-empty --reason")
		})
	}
}
