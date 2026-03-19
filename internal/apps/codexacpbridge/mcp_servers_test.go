package codexacpbridge

import (
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestValidateMCPServersEmpty(t *testing.T) {
	result, err := validateMCPServers(nil)
	if err != nil {
		t.Fatalf("validateMCPServers(nil) error = %v, want nil", err)
	}
	if result != nil {
		t.Fatalf("validateMCPServers(nil) = %v, want nil", result)
	}

	result, err = validateMCPServers([]acp.McpServer{})
	if err != nil {
		t.Fatalf("validateMCPServers([]) error = %v, want nil", err)
	}
	if result != nil {
		t.Fatalf("validateMCPServers([]) = %v, want nil", result)
	}
}

func TestValidateMCPServersStdio(t *testing.T) {
	servers := []acp.McpServer{
		{
			Stdio: &acp.McpServerStdio{
				Name:    "my-server",
				Command: "node",
				Args:    []string{"server.js"},
			},
		},
	}
	result, err := validateMCPServers(servers)
	if err != nil {
		t.Fatalf("validateMCPServers(stdio) error = %v, want nil", err)
	}
	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if _, ok := result["my-server"]; !ok {
		t.Fatalf("result does not contain 'my-server'")
	}
}

func TestValidateMCPServersHTTP(t *testing.T) {
	servers := []acp.McpServer{
		{
			Http: &acp.McpServerHttp{
				Name: "http-server",
				Url:  "http://localhost:8080",
			},
		},
	}
	result, err := validateMCPServers(servers)
	if err != nil {
		t.Fatalf("validateMCPServers(http) error = %v, want nil", err)
	}
	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if _, ok := result["http-server"]; !ok {
		t.Fatalf("result does not contain 'http-server'")
	}
}

func TestValidateMCPServersRejectsSSE(t *testing.T) {
	servers := []acp.McpServer{
		{
			Sse: &acp.McpServerSse{
				Name: "sse-server",
				Url:  "http://localhost:8080/sse",
			},
		},
	}
	_, err := validateMCPServers(servers)
	if err == nil {
		t.Fatal("validateMCPServers(sse) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "sse") {
		t.Fatalf("error = %q, want containing 'sse'", err.Error())
	}
}

func TestValidateMCPServersRejectsNoTransport(t *testing.T) {
	servers := []acp.McpServer{
		{},
	}
	_, err := validateMCPServers(servers)
	if err == nil {
		t.Fatal("validateMCPServers(no transport) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "transport") {
		t.Fatalf("error = %q, want containing 'transport'", err.Error())
	}
}

func TestValidateMCPServersRejectsDuplicateNames(t *testing.T) {
	servers := []acp.McpServer{
		{
			Stdio: &acp.McpServerStdio{Name: "server1", Command: "echo"},
		},
		{
			Stdio: &acp.McpServerStdio{Name: "server1", Command: "echo"},
		},
	}
	_, err := validateMCPServers(servers)
	if err == nil {
		t.Fatal("validateMCPServers(duplicate) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("error = %q, want containing 'duplicated'", err.Error())
	}
}

func TestValidateMCPServersRejectsEmptyName(t *testing.T) {
	servers := []acp.McpServer{
		{
			Stdio: &acp.McpServerStdio{Name: "", Command: "echo"},
		},
	}
	_, err := validateMCPServers(servers)
	if err == nil {
		t.Fatal("validateMCPServers(empty name) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("error = %q, want containing 'name is required'", err.Error())
	}
}
