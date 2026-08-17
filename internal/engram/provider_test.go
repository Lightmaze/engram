package engram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWakeMessagesContinueOriginalSession(t *testing.T) {
	value := Engram{
		ID:        "ancestor",
		Name:      "Ancestor",
		Statement: "This catalog statement must not create the identity.",
		Messages: []Message{
			{Role: "system", Content: "original system"},
			{Role: "user", Content: "原始问题\nwith exact spacing"},
			{Role: "assistant", Content: "original answer"},
		},
	}
	events := []JournalEvent{
		{Kind: "opened", Reason: "design review"},
		{Kind: "wake_result", Scene: "first active-thread slice", Steering: "earlier Engram steering"},
		{Kind: "observation", Role: "assistant", Content: "main Agent outcome"},
	}

	messages := wakeMessages(value, events, "current active-thread slice")
	if len(messages) != 8 {
		t.Fatalf("messages = %d, want 8: %#v", len(messages), messages)
	}
	for i, want := range value.Messages {
		if messages[i].Role != want.Role || messages[i].Content != want.Content {
			t.Fatalf("original message %d changed: got %#v want %#v", i, messages[i], want)
		}
	}
	if strings.Contains(messages[3].Content, value.Statement) || strings.Contains(messages[3].Content, "You are Engram") {
		t.Fatalf("wake boundary recreated identity from a role prompt: %q", messages[3].Content)
	}
	if messages[5] != (providerMessage{Role: "assistant", Content: "earlier Engram steering"}) {
		t.Fatalf("Engram continuation = %#v", messages[5])
	}
	if messages[6].Role != "user" || !strings.Contains(messages[6].Content, "role=assistant") || !strings.Contains(messages[6].Content, "main Agent outcome") {
		t.Fatalf("host observation was confused with Engram speech: %#v", messages[6])
	}
	for _, message := range messages {
		if strings.Contains(message.Content, "<original_context>") {
			t.Fatalf("original conversation was flattened into quoted context: %q", message.Content)
		}
	}
}

func TestOpenAIResponsesProviderSendsMessageSequence(t *testing.T) {
	var received struct {
		Input        []providerMessage `json:"input"`
		Instructions json.RawMessage   `json:"instructions"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"output_text":"historical steering"}`))
	}))
	defer server.Close()

	provider := OpenAIResponsesProvider{Endpoint: server.URL, Model: "test", APIKey: "test", MaxOutputTokens: 64}
	decision, err := provider.Wake(context.Background(), Engram{Messages: []Message{{Role: "user", Content: "original"}, {Role: "assistant", Content: "answer"}}}, nil, "current")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Steering != "historical steering" {
		t.Fatalf("decision = %#v", decision)
	}
	if len(received.Instructions) != 0 {
		t.Fatalf("unexpected identity instructions: %s", received.Instructions)
	}
	if len(received.Input) != 4 || received.Input[0].Role != "user" || received.Input[0].Content != "original" || received.Input[1].Role != "assistant" || received.Input[1].Content != "answer" {
		t.Fatalf("provider input did not preserve the original conversation prefix: %#v", received.Input)
	}
}

func TestProviderSelfFoldKeepsOriginalPrefixAndParsesExactPatch(t *testing.T) {
	var received struct {
		Messages  []providerMessage `json:"messages"`
		MaxTokens int               `json:"max_tokens"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"decision\":\"change\",\"update_intent\":\"carry the observed distinction\",\"posture\":\"Keep observed behavior distinct from source inspection.\",\"what_to_keep\":[\"the original context\"],\"what_to_fold\":[\"the cited result\"],\"what_to_expand_next_time\":[\"the nearest evidence\"],\"activation_boundary_delta\":[\"claims beyond observed evidence\"],\"silence_boundary_delta\":[\"matching evidence is already present\"],\"relation_to_previous_experience\":\"the tool result followed the cited wake\",\"expected_delta\":\"leave unobserved behavior unverified\"}"}}]}`))
	}))
	defer server.Close()

	value := Engram{Messages: []Message{
		{Role: "user", Content: "original question"},
		{Role: "assistant", Content: "original answer"},
	}}
	outcome := JournalEvent{
		ID:      "evt-outcome",
		Kind:    "outcome",
		Content: "The real browser interaction passed.",
		Outcome: &OutcomeRecord{
			WakeEventID:  "evt-wake",
			SourceKind:   "tool_result",
			SourceRef:    "journal-event:evt-tool",
			SourceDigest: "sha256:test",
		},
	}
	provider := ChatProvider{Endpoint: server.URL, Model: "test", APIKey: "test", MaxOutputTokens: 4096}
	patch, err := provider.Fold(context.Background(), value, []JournalEvent{
		{Kind: "opened", Reason: "evidence review"},
		{ID: "evt-wake", Kind: "wake_result", Scene: "Source inspection passed.", Steering: "Wait for the real run."},
		outcome,
	}, outcome)
	if err != nil {
		t.Fatal(err)
	}
	if patch.Decision != "change" || patch.Posture != "Keep observed behavior distinct from source inspection." {
		t.Fatalf("self-fold patch = %#v", patch)
	}
	if len(received.Messages) < 6 || received.Messages[0].Content != "original question" || received.Messages[1].Content != "original answer" {
		t.Fatalf("self-fold provider lost the original message prefix: %#v", received.Messages)
	}
	if received.MaxTokens != 4096 {
		t.Fatalf("DeepSeek-compatible max_tokens = %d, want 4096", received.MaxTokens)
	}
	joined := ""
	for _, message := range received.Messages {
		joined += message.Content + "\n"
	}
	if !strings.Contains(joined, "[Outcome evidence; event_id=evt-outcome") ||
		!strings.Contains(joined, "This is your self-update, not a user review") {
		t.Fatalf("self-fold provider missed the cited outcome or authorship instruction: %#v", received.Messages)
	}
}

func TestSelfFoldParserRejectsUnknownFieldsAndNoChangePosture(t *testing.T) {
	if _, err := selfFoldFromText(`{"decision":"change","update_intent":"x","posture":"y","relation_to_previous_experience":"z","expected_delta":"d","reward":1}`); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
	if _, err := selfFoldFromText(`{"decision":"no_change","update_intent":"x","posture":"model tried to replace it","relation_to_previous_experience":"z"}`); err == nil || !strings.Contains(err.Error(), "cannot replace") {
		t.Fatalf("no_change posture error = %v", err)
	}
}

func TestEmptyProviderWakeIsSilentButEmptyFoldIsInvalid(t *testing.T) {
	tests := []struct {
		name     string
		response string
		provider func(string) Provider
	}{
		{
			name:     "OpenAI Responses",
			response: `{"output_text":""}`,
			provider: func(endpoint string) Provider {
				return &OpenAIResponsesProvider{Endpoint: endpoint, Model: "test", APIKey: "test", MaxOutputTokens: 4096}
			},
		},
		{
			name:     "DeepSeek-compatible chat",
			response: `{"choices":[{"message":{"content":""}}]}`,
			provider: func(endpoint string) Provider {
				return &ChatProvider{Endpoint: endpoint, Model: "test", APIKey: "test", MaxOutputTokens: 4096}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(test.response))
			}))
			defer server.Close()

			provider := test.provider(server.URL)
			decision, err := provider.Wake(context.Background(), Engram{Messages: []Message{{Role: "user", Content: "original"}}}, nil, "current scene")
			if err != nil || decision.State != "silent" || decision.Steering != "" {
				t.Fatalf("empty wake response = %#v, %v; want valid silence", decision, err)
			}
			_, err = provider.Fold(context.Background(), Engram{Messages: []Message{{Role: "user", Content: "original"}}}, nil, JournalEvent{ID: "evt-outcome", Kind: "outcome"})
			if err == nil || !strings.Contains(err.Error(), "no JSON object") {
				t.Fatalf("empty fold response error = %v", err)
			}
		})
	}
}
