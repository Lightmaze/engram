package engram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Provider interface {
	Wake(context.Context, Engram, []JournalEvent, string) (Decision, error)
	Fold(context.Context, Engram, []JournalEvent, JournalEvent) (SelfFoldPatch, error)
}

type ProviderConfig struct {
	Driver          string
	Endpoint        string
	Model           string
	APIKey          string
	MaxOutputTokens int
	AllowRule       bool
}

func NewProvider(config ProviderConfig) (Provider, error) {
	if config.MaxOutputTokens == 0 {
		config.MaxOutputTokens = 4096
	}
	switch config.Driver {
	case "rule":
		if !config.AllowRule {
			return nil, errors.New("rule is a test driver; pass --allow-rule-driver")
		}
		return RuleProvider{}, nil
	case "openai-responses", "":
		if config.Endpoint == "" {
			config.Endpoint = "https://api.openai.com/v1/responses"
		}
		if config.Model == "" {
			return nil, errors.New("OpenAI Responses requires --model or ENGRAM_MODEL")
		}
		if config.APIKey == "" {
			return nil, errors.New("OpenAI Responses requires OPENAI_API_KEY")
		}
		return &OpenAIResponsesProvider{Endpoint: config.Endpoint, Model: config.Model, APIKey: config.APIKey, MaxOutputTokens: config.MaxOutputTokens}, nil
	case "deepseek-chat":
		if config.Endpoint == "" {
			config.Endpoint = "https://api.deepseek.com/chat/completions"
		}
		if config.Model == "" {
			config.Model = "deepseek-chat"
		}
		if config.APIKey == "" {
			return nil, errors.New("DeepSeek Chat requires DEEPSEEK_API_KEY")
		}
		return &ChatProvider{Endpoint: config.Endpoint, Model: config.Model, APIKey: config.APIKey, MaxOutputTokens: config.MaxOutputTokens}, nil
	default:
		return nil, fmt.Errorf("unsupported Engram driver %q", config.Driver)
	}
}

type RuleProvider struct{}

func (RuleProvider) Wake(_ context.Context, value Engram, _ []JournalEvent, _ string) (Decision, error) {
	return Decision{State: "active", Reason: "deterministic conformance response", Steering: value.Statement}, nil
}

func (RuleProvider) Fold(_ context.Context, _ Engram, _ []JournalEvent, outcome JournalEvent) (SelfFoldPatch, error) {
	return SelfFoldPatch{
		Decision:                     "change",
		UpdateIntent:                 "Carry the cited outcome into later judgment.",
		Posture:                      "Treat the cited outcome as scoped external evidence while preserving the complete original history.",
		WhatToKeep:                   []string{"The original Agent-session context remains unchanged."},
		WhatToFold:                   []string{"The exact cited outcome and its relation to the prior intervention."},
		WhatToExpandNextTime:         []string{"Whether a similar scene calls for the same distinction."},
		ActivationBoundaryDelta:      []string{"Notice scenes that reproduce the cited outcome's decision boundary."},
		SilenceBoundaryDelta:         []string{"Remain silent when the cited outcome does not bear on the current scene."},
		RelationToPreviousExperience: "This posture was derived from outcome " + outcome.ID + ".",
		ExpectedDelta:                "A later wake should distinguish cited observation from unsupported confidence.",
	}, nil
}

type OpenAIResponsesProvider struct {
	Endpoint, Model, APIKey string
	MaxOutputTokens         int
}

func (p *OpenAIResponsesProvider) Wake(ctx context.Context, value Engram, events []JournalEvent, scene string) (Decision, error) {
	text, err := p.complete(ctx, wakeMessages(value, events, scene))
	if err != nil {
		return Decision{}, err
	}
	return decisionFromText(text), nil
}

func (p *OpenAIResponsesProvider) Fold(ctx context.Context, value Engram, events []JournalEvent, outcome JournalEvent) (SelfFoldPatch, error) {
	text, err := p.complete(ctx, selfFoldMessages(value, events, outcome))
	if err != nil {
		return SelfFoldPatch{}, err
	}
	return selfFoldFromText(text)
}

func (p *OpenAIResponsesProvider) complete(ctx context.Context, messages []providerMessage) (string, error) {
	payload := map[string]any{
		"model":             p.Model,
		"input":             messages,
		"store":             false,
		"max_output_tokens": p.MaxOutputTokens,
	}
	var response struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct{ Type, Text, Refusal string } `json:"content"`
		} `json:"output"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := postJSON(ctx, p.Endpoint, p.APIKey, payload, &response); err != nil {
		return "", err
	}
	text := strings.TrimSpace(response.OutputText)
	if text == "" {
		for _, item := range response.Output {
			for _, part := range item.Content {
				if part.Type == "output_text" {
					text += part.Text
				}
			}
		}
		text = strings.TrimSpace(text)
	}
	if text == "" && response.Error != nil {
		return "", errors.New("Engram provider error: " + response.Error.Message)
	}
	return text, nil
}

type ChatProvider struct {
	Endpoint, Model, APIKey string
	MaxOutputTokens         int
}

func (p *ChatProvider) Wake(ctx context.Context, value Engram, events []JournalEvent, scene string) (Decision, error) {
	text, err := p.complete(ctx, wakeMessages(value, events, scene))
	if err != nil {
		return Decision{}, err
	}
	return decisionFromText(text), nil
}

func (p *ChatProvider) Fold(ctx context.Context, value Engram, events []JournalEvent, outcome JournalEvent) (SelfFoldPatch, error) {
	text, err := p.complete(ctx, selfFoldMessages(value, events, outcome))
	if err != nil {
		return SelfFoldPatch{}, err
	}
	return selfFoldFromText(text)
}

func (p *ChatProvider) complete(ctx context.Context, messages []providerMessage) (string, error) {
	payload := map[string]any{
		"model":      p.Model,
		"stream":     false,
		"messages":   messages,
		"max_tokens": p.MaxOutputTokens,
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := postJSON(ctx, p.Endpoint, p.APIKey, payload, &response); err != nil {
		return "", err
	}
	if len(response.Choices) == 0 {
		return "", errors.New("Engram provider returned no choices")
	}
	text := strings.TrimSpace(response.Choices[0].Message.Content)
	return text, nil
}

type providerMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func wakeMessages(value Engram, events []JournalEvent, scene string) []providerMessage {
	messages := continuityMessages(value, events)
	messages = append(messages, activeThreadSlice(scene))
	return messages
}

func wakeBoundary(reason string) string {
	text := "[Engram wake boundary]\nThe complete conversation above is your own original Agent-session context. You have now been summoned to observe a slice of a different, currently active Agent thread. Continue thinking from your original context; do not treat the observed thread as your own past. You have no host tools or authority in this accompaniment. Return concise steering when your historical position has something relevant to add. If it does not, reply exactly <silent>."
	if strings.TrimSpace(reason) != "" {
		text += "\nSummon reason: " + reason
	}
	return text
}

func activeThreadSlice(scene string) providerMessage {
	return providerMessage{Role: "user", Content: "[Observed active-thread slice]\n" + scene}
}

func observedThreadMessage(role, content string) providerMessage {
	return providerMessage{Role: "user", Content: fmt.Sprintf("[Observed active-thread message; role=%s]\n%s", role, content)}
}

func decisionFromText(value string) Decision {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "<silent>") {
		return Decision{State: "silent", Reason: "Engram chose silence"}
	}
	return Decision{State: "active", Reason: "Engram responded from its own context", Steering: value}
}

func postJSON(ctx context.Context, endpoint, apiKey string, input, output any) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("provider endpoint must be an http(s) URL")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("provider redirects are disabled") }}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("call Engram provider: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16*1024*1024+1))
	if err != nil {
		return err
	}
	if len(raw) > 16*1024*1024 {
		return errors.New("Engram provider response exceeds 16 MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Engram provider returned %s", response.Status)
	}
	if err := json.Unmarshal(raw, output); err != nil {
		return fmt.Errorf("decode Engram provider response: %w", err)
	}
	return nil
}
