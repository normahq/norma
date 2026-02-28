package llmtools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShellTool(t *testing.T) {
	s := NewShellTool(".")

	t.Run("Allowed command", func(t *testing.T) {
		resp, err := s.Run(nil, ShellArgs{Command: "ls"})
		assert.NoError(t, err)
		assert.Equal(t, 0, resp.ExitCode)
		assert.Contains(t, resp.Stdout, "shell_tool.go")
	})

	t.Run("Allowed command echo", func(t *testing.T) {
		resp, err := s.Run(nil, ShellArgs{Command: "echo hello"})
		assert.NoError(t, err)
		assert.Equal(t, 0, resp.ExitCode)
		assert.Contains(t, resp.Stdout, "hello")
	})

	t.Run("Alias cmd", func(t *testing.T) {
		resp, err := s.Run(nil, ShellArgs{Cmd: "echo alias-cmd"})
		assert.NoError(t, err)
		assert.Equal(t, 0, resp.ExitCode)
		assert.Contains(t, resp.Stdout, "alias-cmd")
	})

	t.Run("Alias input", func(t *testing.T) {
		resp, err := s.Run(nil, ShellArgs{Input: "echo alias-input"})
		assert.NoError(t, err)
		assert.Equal(t, 0, resp.ExitCode)
		assert.Contains(t, resp.Stdout, "alias-input")
	})

	t.Run("Alias shell_command with quotes", func(t *testing.T) {
		resp, err := s.Run(nil, ShellArgs{ShellCommand: "\"echo alias-shell\""})
		assert.NoError(t, err)
		assert.Equal(t, 0, resp.ExitCode)
		assert.Contains(t, resp.Stdout, "alias-shell")
	})

	t.Run("Disallowed command", func(t *testing.T) {
		resp, err := s.Run(nil, ShellArgs{Command: "whoami"})
		assert.NoError(t, err)
		assert.Contains(t, resp.Error, "is not allowed")
	})

	t.Run("Dangerous metacharacter", func(t *testing.T) {
		resp, err := s.Run(nil, ShellArgs{Command: "ls; rm -rf /"})
		assert.NoError(t, err)
		assert.Contains(t, resp.Error, "not allowed")
	})

	t.Run("Stderr capture", func(t *testing.T) {
		resp, err := s.Run(nil, ShellArgs{Command: "ls non_existent_file"})
		assert.NoError(t, err)
		assert.NotEqual(t, 0, resp.ExitCode)
		assert.Contains(t, resp.Stderr, "No such file")
	})
}
