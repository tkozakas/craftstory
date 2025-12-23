package stickers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Provider struct {
	stickersDir string
	count       int
}

func NewProvider(stickersDir string) (*Provider, error) {
	if stickersDir == "" {
		return nil, fmt.Errorf("stickers directory not specified")
	}

	entries, err := os.ReadDir(stickersDir)
	if err != nil {
		return nil, fmt.Errorf("read stickers dir: %w", err)
	}

	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) > 8 && name[:8] == "sticker_" {
			count++
		}
	}

	if count == 0 {
		return nil, fmt.Errorf("no stickers found in %s", stickersDir)
	}

	return &Provider{
		stickersDir: stickersDir,
		count:       count,
	}, nil
}

func (p *Provider) Get(n int) string {
	if n < 1 || n > p.count {
		return ""
	}
	return filepath.Join(p.stickersDir, fmt.Sprintf("sticker_%d.png", n))
}

func (p *Provider) Count() int {
	return p.count
}

func ParseStickerNum(tag string) int {
	if len(tag) < 3 {
		return 0
	}

	if tag[0] == '[' && tag[len(tag)-1] == ']' {
		inner := tag[1 : len(tag)-1]
		if len(inner) >= 2 && inner[0] == 's' {
			if n, err := strconv.Atoi(inner[1:]); err == nil {
				return n
			}
		}
	}

	return 0
}

var stickerTagPattern = regexp.MustCompile(`^\[s(\d+)\]\s*`)

func ExtractStickerFromText(text string) (int, string) {
	matches := stickerTagPattern.FindStringSubmatch(text)
	if len(matches) < 2 {
		return 0, text
	}

	n, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, text
	}

	remaining := strings.TrimPrefix(text, matches[0])
	return n, remaining
}

func DownloadPack(packSlug string, targetDir string, maxStickers int) error {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("create target dir: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	baseURL := fmt.Sprintf("https://s3.getstickerpack.com/storage/uploads/sticker-pack/%s", packSlug)

	downloaded := 0
	for i := 1; i <= maxStickers; i++ {
		url := fmt.Sprintf("%s/sticker_%d.png", baseURL, i)
		targetPath := filepath.Join(targetDir, fmt.Sprintf("sticker_%d.png", i))

		if _, err := os.Stat(targetPath); err == nil {
			downloaded++
			continue
		}

		resp, err := client.Get(url)
		if err != nil {
			if downloaded > 0 {
				break
			}
			return fmt.Errorf("fetch sticker %d: %w", i, err)
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			break
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if downloaded > 0 {
				break
			}
			return fmt.Errorf("sticker %d: status %d", i, resp.StatusCode)
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("read sticker %d: %w", i, err)
		}

		if err := os.WriteFile(targetPath, data, 0644); err != nil {
			return fmt.Errorf("write sticker %d: %w", i, err)
		}

		downloaded++
	}

	if downloaded == 0 {
		return fmt.Errorf("no stickers downloaded from pack %s", packSlug)
	}

	return nil
}
