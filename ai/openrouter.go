package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const openRouterChatEndpoint = "https://openrouter.ai/api/v1/chat/completions"

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type chatRequest struct {
	Model           string        `json:"model"`
	Messages        []chatMessage `json:"messages"`
	Temperature     float64       `json:"temperature"`
	MaxTokens       int           `json:"max_tokens,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
	Stream          bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning"`
		} `json:"message"`
	} `json:"choices"`
}

func callOpenRouterChat(model string, messages []chatMessage, maxTokens int) (string, error) {
	token := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if token == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY is not set")
	}
	if strings.TrimSpace(model) == "" {
		return "", fmt.Errorf("OPENROUTER_TEXT_MODEL is not set")
	}
	requestBody, err := json.Marshal(chatRequest{
		Model: model, Messages: messages, Temperature: 0.4, MaxTokens: maxTokens,
		ReasoningEffort: "none", Stream: false,
	})
	if err != nil {
		return "", fmt.Errorf("encode OpenRouter request: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterChatEndpoint, bytes.NewReader(requestBody))
	if err != nil {
		return "", fmt.Errorf("create OpenRouter request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "http://localhost:8080")
	req.Header.Set("X-Title", "TVILY")
	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("OpenRouter request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("OpenRouter returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	var response chatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&response); err != nil {
		return "", fmt.Errorf("decode OpenRouter response: %w", err)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("OpenRouter returned no chat choices")
	}
	choice := response.Choices[0]
	if strings.TrimSpace(choice.Message.Content) == "" {
		return "", fmt.Errorf("OpenRouter returned an empty chat response (finish_reason=%q, reasoning_chars=%d)",
			choice.FinishReason, len([]rune(choice.Message.Reasoning)))
	}
	return choice.Message.Content, nil
}
