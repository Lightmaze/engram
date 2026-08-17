package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Lightmaze/engram/internal/engram"
)

type silentHookProvider struct{}

func (silentHookProvider) Wake(context.Context, engram.Engram, []engram.JournalEvent, string) (engram.Decision, error) {
	return engram.Decision{State: "silent", Reason: "not relevant", Steering: " \n\t"}, nil
}

func (silentHookProvider) Fold(context.Context, engram.Engram, []engram.JournalEvent, engram.JournalEvent) (engram.SelfFoldPatch, error) {
	return engram.SelfFoldPatch{Decision: "no_change", UpdateIntent: "No durable update.", RelationToPreviousExperience: "The cited outcome does not change this hook test Engram."}, nil
}

func TestGenericHookInjectsRuntimeOwnedEngramAttribution(t *testing.T) {
	hook, runtime := testHook(t, "line one\nline two")
	raw, err := hook.Handle(context.Background(), "generic", []byte(`{"event":"before_prompt","session_id":"host-one","prompt":"Review the current design."}`))
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		AdditionalContext string                   `json:"additional_context"`
		AccompanimentID   string                   `json:"accompaniment_id"`
		Attribution       engram.EngramAttribution `json:"attribution"`
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatal(err)
	}
	if output.Attribution.EngramID != "ancestor" || output.Attribution.Name != "Design Ancestor" {
		t.Fatalf("attribution = %#v", output.Attribution)
	}
	if output.Attribution.AccompanimentID == "" || output.Attribution.AccompanimentID != output.AccompanimentID {
		t.Fatalf("accompaniment attribution = %#v", output)
	}
	if !strings.Contains(output.AdditionalContext, `[Engram: attributed Engram speech]`) ||
		!strings.Contains(output.AdditionalContext, `"engram_id":"ancestor"`) ||
		!strings.Contains(output.AdditionalContext, `"statement":"line one\nline two"`) ||
		!strings.HasSuffix(output.AdditionalContext, "Steering:\nline one\nline two") {
		t.Fatalf("additional_context = %q", output.AdditionalContext)
	}

	pending, found, err := runtime.TakePending("ancestor", "cursor", "missing")
	if err != nil || found || pending.EngramID != "" {
		t.Fatalf("unexpected pending value: %#v, %v, %v", pending, found, err)
	}
}

func TestDirectHookTextNamesTheEngramAndCursorPendingKeepsAttribution(t *testing.T) {
	hook, runtime := testHook(t, "Protect the original design.")
	raw, err := hook.Handle(context.Background(), "codex", []byte(`{"hook_event_name":"UserPromptSubmit","session_id":"codex-one","prompt":"Has the design drifted?"}`))
	if err != nil {
		t.Fatal(err)
	}
	var direct struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(raw, &direct); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(direct.HookSpecificOutput.AdditionalContext, `"name":"Design Ancestor"`) {
		t.Fatalf("direct Hook lost Engram name: %q", direct.HookSpecificOutput.AdditionalContext)
	}

	if _, err := hook.Handle(context.Background(), "cursor", []byte(`{"event":"beforeSubmitPrompt","conversation_id":"cursor-one","prompt":"Has the design drifted?"}`)); err != nil {
		t.Fatal(err)
	}
	pending, found, err := runtime.TakePending("ancestor", "cursor", "cursor-one")
	if err != nil || !found {
		t.Fatalf("pending = %#v, %v, %v", pending, found, err)
	}
	if pending.Attribution.EngramID != "ancestor" || pending.Attribution.Name != "Design Ancestor" || pending.Attribution.AccompanimentID != pending.AccompanimentID {
		t.Fatalf("pending attribution = %#v", pending)
	}
}

func TestCodexHookKeepsAttributionIntactWithinConfiguredLimit(t *testing.T) {
	hook, _ := testHook(t, strings.Repeat("\x01", 16*1024))
	raw, err := hook.Handle(context.Background(), "codex", []byte(`{"hook_event_name":"UserPromptSubmit","session_id":"codex-long","prompt":"Review this."}`))
	if err != nil {
		t.Fatal(err)
	}
	var direct struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(raw, &direct); err != nil {
		t.Fatal(err)
	}
	context := direct.HookSpecificOutput.AdditionalContext
	if len(context) > codexAdditionalContextLimit {
		t.Fatalf("Codex context length = %d, limit = %d", len(context), codexAdditionalContextLimit)
	}
	if !strings.Contains(context, `"engram_id":"ancestor"`) || !strings.Contains(context, `"statement_truncated":true`) || !strings.Contains(context, "Steering:\n") || !strings.HasSuffix(context, truncatedMarkerForTest()) {
		t.Fatalf("bounded Codex context lost attribution or truncation marker: %q", context)
	}
}

func TestCursorSilenceDoesNotLeavePendingSpeech(t *testing.T) {
	journal, err := engram.OpenJournal(filepath.Join(t.TempDir(), "journal"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(engram.CreateRequest{ID: "quiet", Name: "Quiet", Messages: []engram.Message{{Role: "user", Content: "Original"}}}); err != nil {
		t.Fatal(err)
	}
	runtime, err := engram.NewRuntime(journal, silentHookProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.SavePending(engram.PendingSteering{
		ProtocolVersion: "engram/v0",
		AccompanimentID: "acc-stale",
		EngramID:        "quiet",
		Attribution: engram.EngramAttribution{
			EngramID:        "quiet",
			Name:            "Quiet",
			AccompanimentID: "acc-stale",
		},
		Host:     engram.HostRef{Kind: "cursor", SessionID: "cursor-silent"},
		State:    "active",
		Steering: "stale speech",
	}); err != nil {
		t.Fatal(err)
	}
	hook := Hook{Runtime: runtime, EngramID: "quiet"}
	if _, err := hook.Handle(context.Background(), "cursor", []byte(`{"event":"beforeSubmitPrompt","conversation_id":"cursor-silent","prompt":"Unrelated"}`)); err != nil {
		t.Fatal(err)
	}
	pending, found, err := runtime.TakePending("quiet", "cursor", "cursor-silent")
	if err != nil || found || pending.Steering != "" {
		t.Fatalf("silent Cursor wake left pending speech: %#v, %v, %v", pending, found, err)
	}
}

func TestHookObservationPreservesHostMessageRole(t *testing.T) {
	hook, runtime := testHook(t, "Protect the original design.")
	if _, err := hook.Handle(context.Background(), "pi", []byte(`{"event":"before_prompt","session_id":"pi-role","prompt":"Inspect the tool result."}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := hook.Handle(context.Background(), "pi", []byte(`{"event":"after_response","session_id":"pi-role","role":"toolResult","content":"moonmatch help output"}`)); err != nil {
		t.Fatal(err)
	}
	value, err := runtime.Journal.Load("ancestor")
	if err != nil {
		t.Fatal(err)
	}
	events, err := runtime.Journal.Events(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if last.Kind != "observation" || last.Role != "toolResult" || last.Content != "moonmatch help output" {
		t.Fatalf("observation = %#v", last)
	}
}

func TestHookObservationReportsDurabilityFailure(t *testing.T) {
	hook, runtime := testHook(t, "Protect the original design.")
	raw, err := hook.Handle(context.Background(), "pi", []byte(`{"event":"before_prompt","session_id":"pi-failed-observation","prompt":"Inspect the tool result."}`))
	if err != nil {
		t.Fatal(err)
	}
	var opened struct {
		AccompanimentID string `json:"accompaniment_id"`
	}
	if err := json.Unmarshal(raw, &opened); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Release(engram.ReleaseRequest{AccompanimentID: opened.AccompanimentID, Reason: "test closed it early"}); err != nil {
		t.Fatal(err)
	}
	if _, err := hook.Handle(context.Background(), "pi", []byte(`{"event":"after_response","session_id":"pi-failed-observation","role":"toolResult","content":"must not be acknowledged as durable"}`)); err == nil {
		t.Fatal("after_response hid a failed Journal observation")
	}
}

func TestHookSessionEndReportsReleaseFailure(t *testing.T) {
	hook, runtime := testHook(t, "Protect the original design.")
	if _, err := hook.Handle(context.Background(), "pi", []byte(`{"event":"before_prompt","session_id":"pi-release-failure","prompt":"Open accompaniment."}`)); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Journal.WriteIndex("guardians", "pi\x00pi-release-failure\x00ancestor", map[string]string{
		"accompaniment_id": "acc-missing",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := hook.Handle(context.Background(), "pi", []byte(`{"event":"session_end","session_id":"pi-release-failure"}`)); err == nil {
		t.Fatal("session_end hid a failed guardian release")
	}
}

func truncatedMarkerForTest() string {
	return "[Engram steering truncated by host context limit]"
}

func testHook(t *testing.T, statement string) (Hook, *engram.Runtime) {
	t.Helper()
	journal, err := engram.OpenJournal(filepath.Join(t.TempDir(), "journal"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(engram.CreateRequest{
		ID:        "ancestor",
		Name:      "Design Ancestor",
		Statement: statement,
		Messages:  []engram.Message{{Role: "user", Content: "Keep history capable of speaking for itself."}},
	}); err != nil {
		t.Fatal(err)
	}
	runtime, err := engram.NewRuntime(journal, engram.RuleProvider{})
	if err != nil {
		t.Fatal(err)
	}
	return Hook{Runtime: runtime, EngramID: "ancestor"}, runtime
}
