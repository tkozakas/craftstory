package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"craftstory/pkg/config"

	"github.com/charmbracelet/lipgloss"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	authInfoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	authSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	authErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with external services",
	Long:  `Authenticate with YouTube or other services using credentials from .env`,
}

var authYouTubeCmd = &cobra.Command{
	Use:   "youtube",
	Short: "Authenticate with YouTube (OAuth)",
	Long:  `Complete YouTube OAuth flow using credentials from .env file.`,
	RunE:  runAuthYouTube,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check authentication status for all services",
	Long:  `Verify which services are configured and authenticated.`,
	RunE:  runAuthStatus,
}

func init() {
	authCmd.AddCommand(authYouTubeCmd)
	authCmd.AddCommand(authStatusCmd)
	rootCmd.AddCommand(authCmd)
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	cfg, err := config.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println(authInfoStyle.Render("\nService Authentication Status:\n"))

	if cfg.YouTubeClientID != "" && cfg.YouTubeClientSecret != "" {
		if _, err := os.Stat(cfg.YouTubeTokenPath); err == nil {
			fmt.Println(authSuccessStyle.Render("✓ YouTube: authenticated (token exists)"))
		} else {
			fmt.Println(authErrorStyle.Render("✗ YouTube: credentials set, but not authenticated"))
			fmt.Println(authInfoStyle.Render("  Run: craftstory auth youtube"))
		}
	} else {
		fmt.Println(authErrorStyle.Render("✗ YouTube: missing YOUTUBE_CLIENT_ID or YOUTUBE_CLIENT_SECRET"))
	}

	if cfg.GroqAPIKey != "" {
		fmt.Println(authSuccessStyle.Render("✓ Groq: API key configured"))
	} else {
		fmt.Println(authErrorStyle.Render("✗ Groq: missing GROQ_API_KEY"))
	}

	if len(cfg.ElevenLabsAPIKeys) > 0 {
		fmt.Println(authSuccessStyle.Render(fmt.Sprintf("✓ ElevenLabs: %d API key(s) configured", len(cfg.ElevenLabsAPIKeys))))
	} else {
		fmt.Println(authErrorStyle.Render("✗ ElevenLabs: missing ELEVENLABS_API_KEY"))
	}

	if cfg.GoogleSearchAPIKey != "" && cfg.GoogleSearchEngineID != "" {
		fmt.Println(authSuccessStyle.Render("✓ Google Search: configured"))
	} else if cfg.GoogleSearchAPIKey != "" || cfg.GoogleSearchEngineID != "" {
		fmt.Println(authErrorStyle.Render("✗ Google Search: partially configured"))
	} else {
		fmt.Println(authInfoStyle.Render("○ Google Search: not configured (optional)"))
	}

	if cfg.TenorAPIKey != "" {
		fmt.Println(authSuccessStyle.Render("✓ Tenor: API key configured"))
	} else {
		fmt.Println(authInfoStyle.Render("○ Tenor: not configured (optional)"))
	}

	if cfg.TelegramBotToken != "" {
		fmt.Println(authSuccessStyle.Render("✓ Telegram: bot token configured"))
	} else {
		fmt.Println(authInfoStyle.Render("○ Telegram: not configured (optional)"))
	}

	fmt.Println()
	return nil
}

func runAuthYouTube(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	cfg, err := config.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.YouTubeClientID == "" || cfg.YouTubeClientSecret == "" {
		return fmt.Errorf("YOUTUBE_CLIENT_ID and YOUTUBE_CLIENT_SECRET must be set in .env")
	}

	return runYouTubeAuth(cfg.YouTubeClientID, cfg.YouTubeClientSecret, cfg.YouTubeTokenPath)
}

func runYouTubeAuth(clientID, clientSecret, tokenPath string) error {
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))

	oauthConfig := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes: []string{
			"https://www.googleapis.com/auth/youtube.upload",
			"https://www.googleapis.com/auth/youtube",
		},
		RedirectURL: "http://localhost:8085/callback",
	}

	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	listener, err := net.Listen("tcp", ":8085")
	if err != nil {
		return fmt.Errorf("failed to start callback server: %w", err)
	}

	server := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
	}

	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no code in callback")
			_, _ = fmt.Fprintf(w, "<html><body><h1>Error</h1><p>No authorization code received.</p></body></html>")
			return
		}

		codeChan <- code
		_, _ = fmt.Fprintf(w, "<html><body><h1>Success!</h1><p>You can close this window and return to the terminal.</p></body></html>")
	})

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	authURL := oauthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Println(infoStyle.Render("\nOpening browser for YouTube authentication..."))
	fmt.Println(infoStyle.Render("If browser doesn't open, visit:\n" + authURL))

	_ = browser.OpenURL(authURL)

	fmt.Println(infoStyle.Render("\nWaiting for authentication..."))

	select {
	case code := <-codeChan:
		token, err := oauthConfig.Exchange(context.Background(), code)
		if err != nil {
			return fmt.Errorf("failed to exchange code: %w", err)
		}

		data, err := json.MarshalIndent(token, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal token: %w", err)
		}

		if err := os.WriteFile(tokenPath, data, 0600); err != nil {
			return fmt.Errorf("failed to save token: %w", err)
		}

		fmt.Println(successStyle.Render("✓ YouTube authentication complete"))
		fmt.Println(successStyle.Render("  Token saved to: " + tokenPath))
		return nil

	case err := <-errChan:
		return err

	case <-time.After(5 * time.Minute):
		return fmt.Errorf("authentication timed out")
	}
}
