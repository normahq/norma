package main

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/tgbotkit/client"
)

type fakeTelegramMeClient struct {
	resp *client.GetMeResponse
	err  error
}

func (f fakeTelegramMeClient) GetMeWithResponse(_ context.Context, _ ...client.RequestEditorFn) (*client.GetMeResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func TestLoadBotIdentity(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		username := "NormaBot"
		got, err := loadBotIdentity(context.Background(), fakeTelegramMeClient{
			resp: &client.GetMeResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200: &struct {
					Ok     client.GetMe200Ok `json:"ok"`
					Result client.User       `json:"result"`
				}{
					Ok: true,
					Result: client.User{
						FirstName: "Norma Relay",
						Username:  &username,
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("loadBotIdentity(): %v", err)
		}
		if got.username != "NormaBot" {
			t.Fatalf("username = %q, want %q", got.username, "NormaBot")
		}
		if got.name != "Norma Relay" {
			t.Fatalf("name = %q, want %q", got.name, "Norma Relay")
		}
	})

	t.Run("getMe error", func(t *testing.T) {
		_, err := loadBotIdentity(context.Background(), fakeTelegramMeClient{err: errors.New("boom")})
		if err == nil {
			t.Fatal("loadBotIdentity() error = nil, want error")
		}
	})

	t.Run("missing payload", func(t *testing.T) {
		_, err := loadBotIdentity(context.Background(), fakeTelegramMeClient{
			resp: &client.GetMeResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			},
		})
		if err == nil {
			t.Fatal("loadBotIdentity() error = nil, want error")
		}
	})

	t.Run("missing username", func(t *testing.T) {
		_, err := loadBotIdentity(context.Background(), fakeTelegramMeClient{
			resp: &client.GetMeResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200: &struct {
					Ok     client.GetMe200Ok `json:"ok"`
					Result client.User       `json:"result"`
				}{
					Ok: true,
					Result: client.User{
						FirstName: "Norma Relay",
					},
				},
			},
		})
		if err == nil {
			t.Fatal("loadBotIdentity() error = nil, want error")
		}
	})
}

func TestBuildAuthURL(t *testing.T) {
	t.Run("with username", func(t *testing.T) {
		got := buildAuthURL("NormaBot", "token123")
		want := "https://t.me/NormaBot?start=token123"
		if got != want {
			t.Fatalf("buildAuthURL() = %q, want %q", got, want)
		}
	})

	t.Run("fallback username placeholder", func(t *testing.T) {
		got := buildAuthURL(" ", "token123")
		want := "https://t.me/<bot_username>?start=token123"
		if got != want {
			t.Fatalf("buildAuthURL() = %q, want %q", got, want)
		}
	})
}
