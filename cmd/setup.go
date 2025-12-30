package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).MarginBottom(1)
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive setup wizard for Craftstory",
	Long:  `Configure API keys, create directories, and set up the environment for Craftstory.`,
	RunE:  runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	fmt.Println(titleStyle.Render("🎬 Craftstory Setup"))

	steps := []struct {
		name string
		fn   func() error
	}{
		{"Installing tools", installTools},
		{"Creating directories", createDirectories},
		{"Configuring environment", configureEnv},
	}

	for _, step := range steps {
		if err := step.fn(); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}

	return nil
}

func installTools() error {
	if !commandExists("mise") {
		var install bool
		err := huh.NewConfirm().
			Title("mise not found").
			Description("mise is required to manage tool versions. Install it?").
			Affirmative("Yes").
			Negative("No").
			Value(&install).
			Run()

		if err != nil {
			return err
		}

		if !install {
			return fmt.Errorf("mise is required - install from https://mise.jdx.dev")
		}

		if err := installMise(); err != nil {
			return err
		}
	}

	return runWithSpinner("Installing tools via mise", func() error {
		return runSetupCmd("mise", "install")
	})
}

func installMise() error {
	return runWithSpinner("Installing mise", func() error {
		switch runtime.GOOS {
		case "darwin":
			return runSetupCmd("brew", "install", "mise")
		case "linux":
			return runSetupCmd("sh", "-c", "curl -fsSL https://mise.run | sh")
		default:
			return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
		}
	})
}

func createDirectories() error {
	dirs := []string{"assets/backgrounds", "assets/music", "output"}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	fmt.Println(successStyle.Render("✓ Created directories"))
	return nil
}

func configureEnv() error {
	if _, err := os.Stat(".env"); err == nil {
		var overwrite bool
		if err := huh.NewConfirm().
			Title("Found existing .env file").
			Description("Overwrite?").
			Value(&overwrite).
			Run(); err != nil {
			return err
		}
		if !overwrite {
			fmt.Println(infoStyle.Render("Kept existing .env"))
			return nil
		}
	}

	env := make(map[string]string)

	if err := configureGCP(env); err != nil {
		return err
	}

	if err := configureRequiredKeys(env); err != nil {
		return err
	}

	if err := configureOptionalKeys(env); err != nil {
		return err
	}

	return writeEnvFile(env)
}

func configureGCP(env map[string]string) error {
	var setupGCP bool
	if err := huh.NewConfirm().
		Title("Setup Google Cloud?").
		Description("Required for YouTube uploads and image search").
		Value(&setupGCP).
		Run(); err != nil {
		return err
	}

	if !setupGCP {
		return nil
	}

	if !commandExists("gcloud") {
		fmt.Println(warnStyle.Render("gcloud CLI not found - install from https://cloud.google.com/sdk/docs/install"))
		return nil
	}

	project, err := getOrCreateGCPProject()
	if err != nil {
		fmt.Println(warnStyle.Render(fmt.Sprintf("GCP setup skipped: %v", err)))
		return nil
	}

	env["GOOGLE_CLOUD_PROJECT"] = project

	if err := enableGCPAPIs(project); err != nil {
		fmt.Println(warnStyle.Render(fmt.Sprintf("API enablement failed: %v", err)))
	}

	if err := setupYouTubeOAuth(env); err != nil {
		fmt.Println(warnStyle.Render(fmt.Sprintf("YouTube OAuth skipped: %v", err)))
	}

	if err := setupCustomSearch(env); err != nil {
		fmt.Println(warnStyle.Render(fmt.Sprintf("Custom Search skipped: %v", err)))
	}

	return nil
}

func getOrCreateGCPProject() (string, error) {
	existing := getActiveProject()

	var choice string
	options := []huh.Option[string]{
		huh.NewOption("Create new project", "new"),
	}

	if existing != "" {
		options = append([]huh.Option[string]{
			huh.NewOption(fmt.Sprintf("Use current: %s", existing), existing),
		}, options...)
	}

	options = append(options, huh.NewOption("Enter project ID manually", "manual"))

	if err := huh.NewSelect[string]().
		Title("Google Cloud Project").
		Options(options...).
		Value(&choice).
		Run(); err != nil {
		return "", err
	}

	switch choice {
	case "new":
		return createGCPProject()
	case "manual":
		var projectID string
		if err := huh.NewInput().
			Title("Project ID").
			Value(&projectID).
			Run(); err != nil {
			return "", err
		}
		return projectID, nil
	default:
		return choice, nil
	}
}

func getActiveProject() string {
	out, err := exec.Command("gcloud", "config", "get-value", "project").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func createGCPProject() (string, error) {
	var projectID string
	if err := huh.NewInput().
		Title("New Project ID").
		Description("Must be globally unique, 6-30 chars, lowercase letters, digits, hyphens").
		Placeholder("craftstory-12345").
		Value(&projectID).
		Validate(func(s string) error {
			if len(s) < 6 || len(s) > 30 {
				return fmt.Errorf("must be 6-30 characters")
			}
			return nil
		}).
		Run(); err != nil {
		return "", err
	}

	err := runWithSpinner("Creating project", func() error {
		return runSetupCmd("gcloud", "projects", "create", projectID)
	})
	if err != nil {
		return "", err
	}

	_ = runSetupCmd("gcloud", "config", "set", "project", projectID)

	return projectID, nil
}

func enableGCPAPIs(project string) error {
	apis := []string{
		"youtube.googleapis.com",
		"customsearch.googleapis.com",
		"secretmanager.googleapis.com",
	}

	return runWithSpinner("Enabling APIs", func() error {
		args := append([]string{"services", "enable"}, apis...)
		args = append(args, "--project", project)
		return runSetupCmd("gcloud", args...)
	})
}

func setupYouTubeOAuth(env map[string]string) error {
	var setup bool
	if err := huh.NewConfirm().
		Title("Setup YouTube OAuth?").
		Description("Required for uploading videos to YouTube").
		Value(&setup).
		Run(); err != nil || !setup {
		return err
	}

	fmt.Println(infoStyle.Render(`
To create OAuth credentials:
1. Go to https://console.cloud.google.com/apis/credentials
2. Click "Create Credentials" → "OAuth client ID"
3. Choose "Desktop app" as application type
4. Copy the Client ID and Client Secret
`))

	var clientID, clientSecret string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("YouTube Client ID").
				Value(&clientID),
			huh.NewInput().
				Title("YouTube Client Secret").
				EchoMode(huh.EchoModePassword).
				Value(&clientSecret),
		),
	)

	if err := form.Run(); err != nil {
		return err
	}

	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)

	if clientID != "" {
		env["YOUTUBE_CLIENT_ID"] = clientID
	}
	if clientSecret != "" {
		env["YOUTUBE_CLIENT_SECRET"] = clientSecret
	}

	if clientID != "" && clientSecret != "" {
		var authenticate bool
		if err := huh.NewConfirm().
			Title("Authenticate with YouTube now?").
			Description("Opens browser to complete OAuth flow").
			Value(&authenticate).
			Run(); err != nil {
			return err
		}

		if authenticate {
			if err := runYouTubeOAuthFlow(clientID, clientSecret); err != nil {
				fmt.Println(warnStyle.Render(fmt.Sprintf("OAuth flow failed: %v", err)))
				fmt.Println(infoStyle.Render("You can retry later with: craftstory auth youtube"))
			}
		}
	}

	return nil
}

func setupCustomSearch(env map[string]string) error {
	var setup bool
	if err := huh.NewConfirm().
		Title("Setup Google Custom Search?").
		Description("Required for fetching images in videos").
		Value(&setup).
		Run(); err != nil || !setup {
		return err
	}

	fmt.Println(infoStyle.Render(`
To create Custom Search credentials:
1. Go to https://console.cloud.google.com/apis/credentials
2. Click "Create Credentials" → "API Key"
3. Go to https://programmablesearchengine.google.com/
4. Create a search engine and copy the Search Engine ID
`))

	var apiKey, engineID string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Google Search API Key").
				Value(&apiKey),
			huh.NewInput().
				Title("Search Engine ID").
				Value(&engineID),
		),
	)

	if err := form.Run(); err != nil {
		return err
	}

	apiKey = strings.TrimSpace(apiKey)
	engineID = strings.TrimSpace(engineID)

	if apiKey != "" {
		env["GOOGLE_SEARCH_API_KEY"] = apiKey
	}
	if engineID != "" {
		env["GOOGLE_SEARCH_ENGINE_ID"] = engineID
	}

	return nil
}

func configureRequiredKeys(env map[string]string) error {
	var groqKey, elevenKey string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("GROQ API Key").
				Description("https://console.groq.com/keys").
				Value(&groqKey).
				Validate(required("GROQ API Key")),
			huh.NewInput().
				Title("ElevenLabs API Key").
				Description("https://elevenlabs.io/app/settings/api-keys").
				Value(&elevenKey).
				Validate(required("ElevenLabs API Key")),
		),
	)

	if err := form.Run(); err != nil {
		return err
	}

	env["GROQ_API_KEY"] = strings.TrimSpace(groqKey)
	env["ELEVENLABS_API_KEY"] = strings.TrimSpace(elevenKey)
	return nil
}

func configureOptionalKeys(env map[string]string) error {
	if err := configureTenor(env); err != nil {
		return err
	}

	if err := configureTelegram(env); err != nil {
		return err
	}

	return nil
}

func configureTenor(env map[string]string) error {
	var setup bool
	if err := huh.NewConfirm().
		Title("Setup Tenor GIFs?").
		Description("For animated GIF overlays in videos (optional)").
		Value(&setup).
		Run(); err != nil {
		return err
	}

	if !setup {
		return nil
	}

	fmt.Println(infoStyle.Render(`
To get a Tenor API key:
1. Go to https://developers.google.com/tenor/guides/quickstart
2. Create a project and enable Tenor API
3. Copy the API key
`))

	var apiKey string
	if err := huh.NewInput().
		Title("Tenor API Key").
		Value(&apiKey).
		Run(); err != nil {
		return err
	}

	apiKey = strings.TrimSpace(apiKey)
	if apiKey != "" {
		env["TENOR_API_KEY"] = apiKey
	}
	return nil
}

func configureTelegram(env map[string]string) error {
	var setup bool
	if err := huh.NewConfirm().
		Title("Setup Telegram bot?").
		Description("For video approval workflow (optional)").
		Value(&setup).
		Run(); err != nil {
		return err
	}

	if !setup {
		return nil
	}

	var token string
	if err := huh.NewInput().
		Title("Telegram Bot Token").
		Description("Get from @BotFather → https://t.me/BotFather").
		Value(&token).
		Run(); err != nil {
		return err
	}

	token = strings.TrimSpace(token)
	if token != "" {
		env["TELEGRAM_BOT_TOKEN"] = token
	}
	return nil
}

func writeEnvFile(env map[string]string) error {
	f, err := os.Create(".env")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	order := []string{
		"GOOGLE_CLOUD_PROJECT",
		"GROQ_API_KEY",
		"ELEVENLABS_API_KEY",
		"YOUTUBE_CLIENT_ID",
		"YOUTUBE_CLIENT_SECRET",
		"GOOGLE_SEARCH_API_KEY",
		"GOOGLE_SEARCH_ENGINE_ID",
		"TENOR_API_KEY",
		"TELEGRAM_BOT_TOKEN",
	}

	for _, key := range order {
		if val, ok := env[key]; ok && val != "" {
			_, _ = fmt.Fprintf(f, "%s=%s\n", key, val)
		}
	}

	fmt.Println(successStyle.Render("✓ Created .env file"))
	printNextSteps()
	return nil
}

func printNextSteps() {
	fmt.Println()
	fmt.Println(titleStyle.Render("Next steps:"))
	fmt.Println("  1. Add background videos to: assets/backgrounds/")
	fmt.Println("  2. Add music (optional) to: assets/music/")
	fmt.Println("  3. Run: craftstory once -t \"your topic\"")
}

func required(field string) func(string) error {
	return func(s string) error {
		if s == "" {
			return fmt.Errorf("%s is required", field)
		}
		return nil
	}
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runSetupCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %s", err, stderr.String())
	}
	return nil
}

func runWithSpinner(title string, fn func() error) error {
	var err error
	_ = spinner.New().
		Title(title).
		Action(func() { err = fn() }).
		Run()
	if err != nil {
		return err
	}
	fmt.Println(successStyle.Render("✓ " + title))
	return nil
}

const youtubeTokenPath = "./youtube_token.json"

func runYouTubeOAuthFlow(clientID, clientSecret string) error {
	return runYouTubeAuth(clientID, clientSecret, youtubeTokenPath)
}
