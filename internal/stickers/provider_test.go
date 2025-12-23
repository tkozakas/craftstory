package stickers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(string)
		wantCount int
		wantErr   bool
	}{
		{
			name: "validStickers",
			setup: func(dir string) {
				_ = os.WriteFile(filepath.Join(dir, "sticker_1.png"), []byte("fake"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "sticker_2.png"), []byte("fake"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "sticker_3.png"), []byte("fake"), 0644)
			},
			wantCount: 3,
			wantErr:   false,
		},
		{
			name: "noStickers",
			setup: func(dir string) {
				_ = os.WriteFile(filepath.Join(dir, "other.png"), []byte("fake"), 0644)
			},
			wantCount: 0,
			wantErr:   true,
		},
		{
			name:      "emptyDir",
			setup:     func(dir string) {},
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(dir)

			provider, err := NewProvider(dir)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && provider.Count() != tt.wantCount {
				t.Errorf("Count() = %d, want %d", provider.Count(), tt.wantCount)
			}
		})
	}
}

func TestNewProviderEmptyPath(t *testing.T) {
	_, err := NewProvider("")
	if err == nil {
		t.Error("NewProvider(\"\") should return error")
	}
}

func TestNewProviderNonExistentDir(t *testing.T) {
	_, err := NewProvider("/nonexistent/path")
	if err == nil {
		t.Error("NewProvider with nonexistent dir should return error")
	}
}

func TestProviderGet(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "sticker_1.png"), []byte("fake"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "sticker_2.png"), []byte("fake"), 0644)

	provider, err := NewProvider(dir)
	if err != nil {
		t.Fatalf("NewProvider failed: %v", err)
	}

	tests := []struct {
		name string
		n    int
		want string
	}{
		{"validFirst", 1, filepath.Join(dir, "sticker_1.png")},
		{"validSecond", 2, filepath.Join(dir, "sticker_2.png")},
		{"zeroIndex", 0, ""},
		{"negativeIndex", -1, ""},
		{"outOfRange", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := provider.Get(tt.n)
			if got != tt.want {
				t.Errorf("Get(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestParseStickerNum(t *testing.T) {
	tests := []struct {
		tag  string
		want int
	}{
		{"[s1]", 1},
		{"[s5]", 5},
		{"[s12]", 12},
		{"[s99]", 99},
		{"[s0]", 0},
		{"", 0},
		{"s1", 0},
		{"[1]", 0},
		{"[sx]", 0},
		{"[s]", 0},
		{"hello", 0},
		{"[]", 0},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			got := ParseStickerNum(tt.tag)
			if got != tt.want {
				t.Errorf("ParseStickerNum(%q) = %d, want %d", tt.tag, got, tt.want)
			}
		})
	}
}

func TestExtractStickerFromText(t *testing.T) {
	tests := []struct {
		text     string
		wantNum  int
		wantText string
	}{
		{"[s1] Hello world", 1, "Hello world"},
		{"[s5] Test message", 5, "Test message"},
		{"[s12]  Extra spaces", 12, "Extra spaces"},
		{"No sticker here", 0, "No sticker here"},
		{"", 0, ""},
		{"[s1]NoSpace", 1, "NoSpace"},
		{"Hello [s1] middle", 0, "Hello [s1] middle"},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			gotNum, gotText := ExtractStickerFromText(tt.text)
			if gotNum != tt.wantNum {
				t.Errorf("ExtractStickerFromText(%q) num = %d, want %d", tt.text, gotNum, tt.wantNum)
			}
			if gotText != tt.wantText {
				t.Errorf("ExtractStickerFromText(%q) text = %q, want %q", tt.text, gotText, tt.wantText)
			}
		})
	}
}

func TestDownloadPack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/storage/uploads/sticker-pack/test-pack/sticker_1.png" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("fake png data"))
			return
		}
		if r.URL.Path == "/storage/uploads/sticker-pack/test-pack/sticker_2.png" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("fake png data 2"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Run("successfulDownload", func(t *testing.T) {
		dir := t.TempDir()
		err := downloadPackFromURL(server.URL+"/storage/uploads/sticker-pack", "test-pack", dir, 5)
		if err != nil {
			t.Errorf("DownloadPack failed: %v", err)
		}

		if _, err := os.Stat(filepath.Join(dir, "sticker_1.png")); err != nil {
			t.Error("sticker_1.png not found")
		}
		if _, err := os.Stat(filepath.Join(dir, "sticker_2.png")); err != nil {
			t.Error("sticker_2.png not found")
		}
	})

	t.Run("skipExisting", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "sticker_1.png"), []byte("existing"), 0644)

		err := downloadPackFromURL(server.URL+"/storage/uploads/sticker-pack", "test-pack", dir, 5)
		if err != nil {
			t.Errorf("DownloadPack failed: %v", err)
		}

		data, _ := os.ReadFile(filepath.Join(dir, "sticker_1.png"))
		if string(data) != "existing" {
			t.Error("existing file was overwritten")
		}
	})
}

func downloadPackFromURL(baseURL, packSlug, targetDir string, maxStickers int) error {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	client := &http.Client{}
	downloaded := 0

	for i := 1; i <= maxStickers; i++ {
		url := baseURL + "/" + packSlug + "/sticker_" + itoa(i) + ".png"
		targetPath := filepath.Join(targetDir, "sticker_"+itoa(i)+".png")

		if _, err := os.Stat(targetPath); err == nil {
			downloaded++
			continue
		}

		resp, err := client.Get(url)
		if err != nil {
			if downloaded > 0 {
				break
			}
			return err
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			break
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			break
		}

		data := make([]byte, 1024)
		n, _ := resp.Body.Read(data)
		resp.Body.Close()

		if err := os.WriteFile(targetPath, data[:n], 0644); err != nil {
			return err
		}
		downloaded++
	}

	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}

func TestProviderCount(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "sticker_1.png"), []byte("fake"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "sticker_2.png"), []byte("fake"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "sticker_3.png"), []byte("fake"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "other.png"), []byte("fake"), 0644)

	provider, _ := NewProvider(dir)
	if provider.Count() != 3 {
		t.Errorf("Count() = %d, want 3", provider.Count())
	}
}
