package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ClaudeClient handles communication with the Anthropic API
type ClaudeClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

type ClaudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ClaudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system,omitempty"`
	Messages  []ClaudeMessage `json:"messages"`
}

type ClaudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func NewClaudeClient(apiKey string) *ClaudeClient {
	return &ClaudeClient{
		apiKey:  apiKey,
		baseURL: "https://api.anthropic.com/v1/messages",
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *ClaudeClient) Complete(prompt, systemPrompt string) (string, error) {
	return c.CompleteModel(claudeModel(), prompt, systemPrompt)
}

func (c *ClaudeClient) CompleteModel(model, prompt, systemPrompt string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("API key not configured")
	}

	messages := []ClaudeMessage{
		{Role: "user", Content: prompt},
	}

	reqBody := ClaudeRequest{
		Model:     model,
		MaxTokens: 1024,
		System:    systemPrompt,
		Messages:  messages,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", c.baseURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API error: %s", string(body))
	}

	var claudeResp ClaudeResponse
	err = json.Unmarshal(body, &claudeResp)
	if err != nil {
		return "", err
	}

	// The reply can start with a non-text block (such as the model's reasoning), so join the text blocks.
	var text string
	for _, blk := range claudeResp.Content {
		if blk.Type == "text" {
			text += blk.Text
		}
	}
	if strings.TrimSpace(text) != "" {
		return text, nil
	}

	return "", fmt.Errorf("empty response from API")
}

func (c *ClaudeClient) CompleteWithSystem(systemPrompt, userPrompt string) (string, error) {
	return c.Complete(userPrompt, systemPrompt)
}

// claudeModel is the cloud model used when an API key is set. Override with SOCRATEAN_MODEL.
func claudeModel() string {
	if m := os.Getenv("SOCRATEAN_MODEL"); m != "" {
		return m
	}
	return "claude-sonnet-5-5"
}
