package uploader

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestNewYouTubeAuth(t *testing.T) {
	auth := NewYouTubeAuth("client-id", "client-secret", "/tmp/token.json")

	if auth == nil {
		t.Fatal("NewYouTubeAuth() returned nil")
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

func TestNewYouTubeUploader(t *testing.T) {
	auth := NewYouTubeAuth("id", "secret", "/tmp/token.json")
	uploader := NewYouTubeUploader(auth)

	if uploader == nil {
		t.Fatal("NewYouTubeUploader() returned nil")
	}
	if uploader.auth != auth {
		t.Error("NewYouTubeUploader() did not set auth correctly")
	}
}

func TestYouTubePlatform(t *testing.T) {
	uploader := NewYouTubeUploader(nil)
	if got := uploader.Platform(); got != youtubePlatform {
		t.Errorf("Platform() = %q, want %q", got, youtubePlatform)
	}
}

func TestYouTubeUploaderAuth(t *testing.T) {
	auth := NewYouTubeAuth("id", "secret", "/tmp/token.json")
	uploader := NewYouTubeUploader(auth)

	if uploader.Auth() != auth {
		t.Error("Auth() did not return the correct auth")
	}
}

func TestYouTubeAuthGetAuthURL(t *testing.T) {
	auth := NewYouTubeAuth("client-id", "client-secret", "/tmp/token.json")
	url := auth.GetAuthURL()

	if url == "" {
		t.Error("GetAuthURL() returned empty string")
	}
	if len(url) < 50 {
		t.Error("GetAuthURL() returned suspiciously short URL")
	}
}

func TestYouTubeAuthLoadToken(t *testing.T) {
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
				// Don't create any file
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

			auth := NewYouTubeAuth("id", "secret", tokenPath)
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

func TestYouTubeAuthSaveToken(t *testing.T) {
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

			auth := NewYouTubeAuth("id", "secret", tokenPath)
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

func TestYouTubeAuthSaveTokenInvalidPath(t *testing.T) {
	auth := NewYouTubeAuth("id", "secret", "/nonexistent/dir/token.json")
	auth.token = &oauth2.Token{AccessToken: "test"}

	err := auth.SaveToken()
	if err == nil {
		t.Error("SaveToken() should return error for invalid path")
	}
}

func TestYouTubeAuthIsAuthenticated(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T, auth *YouTubeAuth)
		want      bool
	}{
		{
			name: "noToken",
			setupFunc: func(t *testing.T, auth *YouTubeAuth) {
				// No token set, no file exists
			},
			want: false,
		},
		{
			name: "validToken",
			setupFunc: func(t *testing.T, auth *YouTubeAuth) {
				auth.token = &oauth2.Token{
					AccessToken: "valid-token",
					Expiry:      time.Now().Add(time.Hour),
				}
			},
			want: true,
		},
		{
			name: "expiredToken",
			setupFunc: func(t *testing.T, auth *YouTubeAuth) {
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

			auth := NewYouTubeAuth("id", "secret", tokenPath)
			tt.setupFunc(t, auth)

			got := auth.IsAuthenticated()
			if got != tt.want {
				t.Errorf("IsAuthenticated() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestYouTubeAuthClient(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T, auth *YouTubeAuth, path string)
		wantErr   bool
	}{
		{
			name: "withExistingToken",
			setupFunc: func(t *testing.T, auth *YouTubeAuth, path string) {
				auth.token = &oauth2.Token{
					AccessToken: "test-token",
					Expiry:      time.Now().Add(time.Hour),
				}
			},
			wantErr: false,
		},
		{
			name: "loadTokenFromFile",
			setupFunc: func(t *testing.T, auth *YouTubeAuth, path string) {
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
			setupFunc: func(t *testing.T, auth *YouTubeAuth, path string) {
				// No token set, no file
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tokenPath := filepath.Join(tmpDir, "token.json")

			auth := NewYouTubeAuth("id", "secret", tokenPath)
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

func TestYouTubeUploaderUploadNoAuth(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token.json")

	auth := NewYouTubeAuth("id", "secret", tokenPath)
	uploader := NewYouTubeUploader(auth)

	ctx := context.Background()
	_, err := uploader.Upload(ctx, UploadRequest{
		FilePath: "/tmp/test.mp4",
		Title:    "Test",
	})

	if err == nil {
		t.Error("Upload() should fail without auth")
	}
}

func TestYouTubeUploaderUploadBadFile(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token.json")

	token := &oauth2.Token{
		AccessToken: "test-token",
		Expiry:      time.Now().Add(time.Hour),
	}
	tokenData, _ := json.Marshal(token)
	_ = os.WriteFile(tokenPath, tokenData, 0600)

	auth := NewYouTubeAuth("id", "secret", tokenPath)
	uploader := NewYouTubeUploader(auth)

	ctx := context.Background()
	_, err := uploader.Upload(ctx, UploadRequest{
		FilePath: "/nonexistent/video.mp4",
		Title:    "Test",
	})

	if err == nil {
		t.Error("Upload() should fail with nonexistent file")
	}
}

func TestYouTubeUploaderSetPrivacyNoAuth(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token.json")

	auth := NewYouTubeAuth("id", "secret", tokenPath)
	uploader := NewYouTubeUploader(auth)

	ctx := context.Background()
	err := uploader.SetPrivacy(ctx, "video-id", "public")

	if err == nil {
		t.Error("SetPrivacy() should fail without auth")
	}
}
