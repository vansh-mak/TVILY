package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/disintegration/imaging"
)

const openRouterImageEndpoint = "https://openrouter.ai/api/v1/images"

type openRouterImageResponse struct {
	Data []struct {
		B64JSON   string `json:"b64_json"`
		MediaType string `json:"media_type"`
	} `json:"data"`
}

func GenerateThemeBackground(theme, pageDirection, outputPath string) error {
	theme = strings.TrimSpace(theme)
	pageDirection = strings.TrimSpace(pageDirection)
	if theme == "" {
		return fmt.Errorf("magazine theme is required")
	}
	if pageDirection == "" {
		pageDirection = theme
	}
	if utf8.RuneCountInString(theme) > 3000 {
		return fmt.Errorf("magazine theme must be 3000 characters or fewer")
	}
	token := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if token == "" {
		return fmt.Errorf("OPENROUTER_API_KEY is not set")
	}
	model := strings.TrimSpace(os.Getenv("OPENROUTER_IMAGE_MODEL"))
	if model == "" {
		model = strings.TrimSpace(os.Getenv("OPENROUTER_MODEL"))
	}
	if model == "" {
		return fmt.Errorf("OPENROUTER_IMAGE_MODEL is not set")
	}

	prompt := fmt.Sprintf(`Create a full-bleed portrait visual background for one page of a premium photo magazine.
Overall magazine theme: %q.
This page's planned background direction: %q.
Use the page direction for this page while keeping its palette and atmosphere coherent with the overall magazine theme.
Create only atmospheric background art: no words, letters, numbers, logos, frames, panels, or recognizable photographs.
Keep the composition low contrast and uncluttered, with subtle texture and open areas so foreground photos remain clear.`, theme, pageDirection)
	body, err := json.Marshal(map[string]any{
		"model": model, "prompt": prompt, "resolution": "1K", "aspect_ratio": "3:4", "output_format": "png", "n": 1,
	})
	if err != nil {
		return fmt.Errorf("encode OpenRouter image request: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterImageEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create OpenRouter image request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "http://localhost:8080")
	req.Header.Set("X-Title", "TVILY")
	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("OpenRouter image request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("OpenRouter image generation returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	var response openRouterImageResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 30<<20)).Decode(&response); err != nil {
		return fmt.Errorf("decode OpenRouter image response: %w", err)
	}
	if len(response.Data) == 0 || strings.TrimSpace(response.Data[0].B64JSON) == "" {
		return fmt.Errorf("OpenRouter returned no generated image")
	}
	imageBytes, err := base64.StdEncoding.DecodeString(response.Data[0].B64JSON)
	if err != nil {
		return fmt.Errorf("decode OpenRouter generated image: %w", err)
	}
	generated, err := imaging.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return fmt.Errorf("decode generated background: %w", err)
	}
	if err := imaging.Save(generated, outputPath); err != nil {
		return fmt.Errorf("save generated background: %w", err)
	}
	return nil
}
