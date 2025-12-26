package youtube

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"craftstory/internal/distribution"
)

func TestNewAuth(t *testing.T) {
	auth := NewAuth("client-id", "client-secret", "/tmp/token.json")

	if auth == nil {
		t.Fatal("NewAuth() returned nil")
	}
	if auth.config.ClientID != "client-id" {
		t.Errorf("ClientID = %q, want %q", auth.config.ClientID, "client-id")
	}
	if auth.config.ClientSecret != "client-secret" {
		t.Errorf("ClientSecret = %q, want %q", auth.config.ClientSecret, "client-secret")
	}
	if auth.tokenPath != "/tmp/token.json" {
		t.Errorf("tokenPath = %q, want %q", auth.tokenPath, "/tmp/token.json")
	}
}

func TestNewClient(t *testing.T) {
	auth := NewAuth("id", "secret", "/tmp/token.json")
	client := NewClient(auth)

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	if client.auth != auth {
		t.Error("NewClient() did not set auth correctly")
	}
}

func TestPlatform(t *testing.T) {
	client := NewClient(nil)
	if got := client.Platform(); got != platform {
		t.Errorf("Platform() = %q, want %q", got, platform)
	}
}

func TestClientAuth(t *testing.T) {
	auth := NewAuth("id", "secret", "/tmp/token.json")
	client := NewClient(auth)

	if client.Auth() != auth {
		t.Error("Auth() did not return the correct auth")
	}
}

func TestAuthGetAuthURL(t *testing.T) {
	auth := NewAuth("client-id", "client-secret", "/tmp/token.json")
	url := auth.GetAuthURL()

	if url == "" {
		t.Error("GetAuthURL() returned empty string")
	}
	if len(url) < 50 {
		t.Error("GetAuthURL() returned suspiciously short URL")
	}
}

func TestAuthLoadToken(t *testing.T) {
	tests := []struct {
		name      string
		token     *oauth2.Token
		wantErr   bool
		setupFunc func(t *testing.T, path string)
	}{
		{
			name: "validToken",
			token: &oauth2.Token{
				AccessToken:  "test-access-token",
				TokenType:    "Bearer",
				RefreshToken: "test-refresh-token",
				Expiry:       time.Now().Add(time.Hour),
			},
			wantErr: false,
		},
		{
			name:    "missingFile",
			wantErr: true,
			setupFunc: func(t *testing.T, path string) {
			},
		},
		{
			name:    "invalidJSON",
			wantErr: true,
			setupFunc: func(t *testing.T, path string) {
				_ = os.WriteFile(path, []byte("not valid json"), 0600)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tokenPath := filepath.Join(tmpDir, "token.json")

			if tt.token != nil {
				tokenData, _ := json.Marshal(tt.token)
				_ = os.WriteFile(tokenPath, tokenData, 0600)
			} else if tt.setupFunc != nil {
				tt.setupFunc(t, tokenPath)
			}

			auth := NewAuth("id", "secret", tokenPath)
			err := auth.LoadToken()

			if (err != nil) != tt.wantErr {
				t.Errorf("LoadToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && auth.token == nil {
				t.Error("LoadToken() did not set token")
			}
		})
	}
}

func TestAuthSaveToken(t *testing.T) {
	tests := []struct {
		name    string
		token   *oauth2.Token
		wantErr bool
	}{
		{
			name: "validToken",
			token: &oauth2.Token{
				AccessToken:  "save-test-token",
				TokenType:    "Bearer",
				RefreshToken: "save-refresh-token",
				Expiry:       time.Now().Add(time.Hour),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tokenPath := filepath.Join(tmpDir, "token.json")

			auth := NewAuth("id", "secret", tokenPath)
			auth.token = tt.token

			err := auth.SaveToken()

			if (err != nil) != tt.wantErr {
				t.Errorf("SaveToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				data, err := os.ReadFile(tokenPath)
				if err != nil {
					t.Fatalf("failed to read saved token: %v", err)
				}

				var savedToken oauth2.Token
				if err := json.Unmarshal(data, &savedToken); err != nil {
					t.Fatalf("failed to unmarshal saved token: %v", err)
				}

				if savedToken.AccessToken != tt.token.AccessToken {
					t.Errorf("saved AccessToken = %q, want %q", savedToken.AccessToken, tt.token.AccessToken)
				}
			}
		})
	}
}

func TestAuthSaveTokenInvalidPath(t *testing.T) {
	auth := NewAuth("id", "secret", "/nonexistent/dir/token.json")
	auth.token = &oauth2.Token{AccessToken: "test"}

	err := auth.SaveToken()
	if err == nil {
		t.Error("SaveToken() should return error for invalid path")
	}
}

func TestAuthIsAuthenticated(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T, auth *Auth)
		want      bool
	}{
		{
			name: "noToken",
			setupFunc: func(t *testing.T, auth *Auth) {
			},
			want: false,
		},
		{
			name: "validToken",
			setupFunc: func(t *testing.T, auth *Auth) {
				auth.token = &oauth2.Token{
					AccessToken: "valid-token",
					Expiry:      time.Now().Add(time.Hour),
				}
			},
			want: true,
		},
		{
			name: "expiredToken",
			setupFunc: func(t *testing.T, auth *Auth) {
				auth.token = &oauth2.Token{
					AccessToken: "expired-token",
					Expiry:      time.Now().Add(-time.Hour),
				}
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tokenPath := filepath.Join(tmpDir, "token.json")

			auth := NewAuth("id", "secret", tokenPath)
			tt.setupFunc(t, auth)

			got := auth.IsAuthenticated()
			if got != tt.want {
				t.Errorf("IsAuthenticated() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthClient(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T, auth *Auth, path string)
		wantErr   bool
	}{
		{
			name: "withExistingToken",
			setupFunc: func(t *testing.T, auth *Auth, path string) {
				auth.token = &oauth2.Token{
					AccessToken: "test-token",
					Expiry:      time.Now().Add(time.Hour),
				}
			},
			wantErr: false,
		},
		{
			name: "loadTokenFromFile",
			setupFunc: func(t *testing.T, auth *Auth, path string) {
				token := &oauth2.Token{
					AccessToken: "file-token",
					Expiry:      time.Now().Add(time.Hour),
				}
				data, _ := json.Marshal(token)
				_ = os.WriteFile(path, data, 0600)
			},
			wantErr: false,
		},
		{
			name: "noTokenAvailable",
			setupFunc: func(t *testing.T, auth *Auth, path string) {
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tokenPath := filepath.Join(tmpDir, "token.json")

			auth := NewAuth("id", "secret", tokenPath)
			tt.setupFunc(t, auth, tokenPath)

			ctx := context.Background()
			client, err := auth.Client(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("Client() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && client == nil {
				t.Error("Client() returned nil client")
			}
		})
	}
}

func TestClientUploadNoAuth(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token.json")

	auth := NewAuth("id", "secret", tokenPath)
	client := NewClient(auth)

	ctx := context.Background()
	_, err := client.Upload(ctx, distribution.UploadRequest{
		FilePath: "/tmp/test.mp4",
		Title:    "Test",
	})

	if err == nil {
		t.Error("Upload() should fail without auth")
	}
}

func TestClientUploadBadFile(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token.json")

	token := &oauth2.Token{
		AccessToken: "test-token",
		Expiry:      time.Now().Add(time.Hour),
	}
	tokenData, _ := json.Marshal(token)
	_ = os.WriteFile(tokenPath, tokenData, 0600)

	auth := NewAuth("id", "secret", tokenPath)
	client := NewClient(auth)

	ctx := context.Background()
	_, err := client.Upload(ctx, distribution.UploadRequest{
		FilePath: "/nonexistent/video.mp4",
		Title:    "Test",
	})

	if err == nil {
		t.Error("Upload() should fail with nonexistent file")
	}
}

func TestClientSetPrivacyNoAuth(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token.json")

	auth := NewAuth("id", "secret", tokenPath)
	client := NewClient(auth)

	ctx := context.Background()
	err := client.SetPrivacy(ctx, "video-id", "public")

	if err == nil {
		t.Error("SetPrivacy() should fail without auth")
	}
}
