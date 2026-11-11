package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type LLMClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewLLMClient(baseURL, apiKey, model string) *LLMClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &LLMClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *LLMClient) IsConfigured() bool {
	return c != nil && c.baseURL != "" && c.apiKey != "" && c.model != ""
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string            `json:"model"`
	Temperature    float64           `json:"temperature"`
	ResponseFormat map[string]string `json:"response_format"`
	Messages       []chatMessage     `json:"messages"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

// ChatJSON issues a completion request enforcing JSON output format.
func (c *LLMClient) ChatJSON(ctx context.Context, systemPrompt, userPrompt string, target interface{}) error {
	if !c.IsConfigured() {
		return errors.New("llm client not configured")
	}

	// Truncate user prompt to 1000 characters (runes, never mid-UTF-8) as per spec §4.6
	if r := []rune(userPrompt); len(r) > 1000 {
		userPrompt = string(r[:1000])
	}

	reqBody := chatRequest{
		Model:          c.model,
		Temperature:    0.0,
		ResponseFormat: map[string]string{"type": "json_object"},
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	endpoint := c.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.httpClient.Do(httpReq)
	elapsed := time.Since(start)

	if err != nil {
		slog.Warn("llm request failed", "latency_ms", elapsed.Milliseconds(), "error", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Never log the provider's response body: it may echo user text (BR/US-14 AC3).
		slog.Warn("llm non-200 response", "status", resp.StatusCode, "latency_ms", elapsed.Milliseconds())
		return fmt.Errorf("llm returned status %d", resp.StatusCode)
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return fmt.Errorf("decode llm response wrapper: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return errors.New("llm returned no choices")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	// Strip markdown fences ```json ... ``` if model returned them
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if len(lines) > 0 && strings.HasPrefix(lines[len(lines)-1], "```") {
				lines = lines[:len(lines)-1]
			}
			content = strings.Join(lines, "\n")
		}
	}

	dec := json.NewDecoder(strings.NewReader(content))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("unmarshal llm json output: %w", err)
	}

	slog.Info("llm call completed", "model", c.model, "latency_ms", elapsed.Milliseconds())
	return nil
}
