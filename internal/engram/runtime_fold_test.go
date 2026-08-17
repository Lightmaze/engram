package engram

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type foldResponse struct {
	Patch SelfFoldPatch
	Err   error
}

type recordedFold struct {
	Engram  Engram
	Events  []JournalEvent
	Outcome JournalEvent
}

type foldingTestProvider struct {
	WakeSteering string
	WakeCalls    []recordedWake
	FoldCalls    []recordedFold
	Responses    []foldResponse
}

func (provider *foldingTestProvider) Wake(_ context.Context, value Engram, events []JournalEvent, scene string) (Decision, error) {
	value.Messages = append([]Message(nil), value.Messages...)
	provider.WakeCalls = append(provider.WakeCalls, recordedWake{Engram: value, Events: append([]JournalEvent(nil), events...), Scene: scene})
	return Decision{State: "active", Reason: "fold test wake", Steering: provider.WakeSteering}, nil
}

func (provider *foldingTestProvider) Fold(_ context.Context, value Engram, events []JournalEvent, outcome JournalEvent) (SelfFoldPatch, error) {
	value.Messages = append([]Message(nil), value.Messages...)
	provider.FoldCalls = append(provider.FoldCalls, recordedFold{Engram: value, Events: append([]JournalEvent(nil), events...), Outcome: outcome})
	if len(provider.Responses) == 0 {
		return SelfFoldPatch{Decision: "no_change", UpdateIntent: "No queued test change.", RelationToPreviousExperience: "The test provider found no additional durable change."}, nil
	}
	response := provider.Responses[0]
	provider.Responses = provider.Responses[1:]
	return response.Patch, response.Err
}

func TestOutcomeAppliesEngramSelfFoldAndNextRuntimeLoadsIt(t *testing.T) {
	root := t.TempDir()
	journal, err := OpenJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	original := []Message{
		{ID: "origin-user", Role: "user", Content: "A source inspection is not always the same as a running-system observation."},
		{ID: "origin-assistant", Role: "assistant", Content: "I should preserve that distinction when evidence levels diverge."},
	}
	if _, err := journal.Create(CreateRequest{ID: "evidence-ancestor", Name: "Evidence Ancestor", Messages: original}); err != nil {
		t.Fatal(err)
	}
	engramPath := filepath.Join(root, "engrams", "evidence-ancestor", "engram.json")
	before, err := os.ReadFile(engramPath)
	if err != nil {
		t.Fatal(err)
	}
	patch := SelfFoldPatch{
		Decision:                     "change",
		UpdateIntent:                 "Keep claims at the level of evidence actually observed.",
		Posture:                      "When a final behavior is claimed, distinguish source inspection and unit checks from the closest real execution or visual observation; leave unobserved layers explicitly unverified.",
		WhatToKeep:                   []string{"Preserve exact original context and cited outcome."},
		WhatToFold:                   []string{"The test result corrected confidence based only on source inspection."},
		WhatToExpandNextTime:         []string{"Ask which evidence is closest to the claimed behavior."},
		ActivationBoundaryDelta:      []string{"Speak when a claim outruns the observed evidence layer."},
		SilenceBoundaryDelta:         []string{"Do not repeat the distinction after matching evidence is already present."},
		RelationToPreviousExperience: "The cited tool result follows the wake that questioned evidence level.",
		ExpectedDelta:                "On a similar scene, mark the final behavior unverified until the closest observation exists.",
	}
	providerA := &foldingTestProvider{WakeSteering: "Do not promote source inspection into a runtime claim.", Responses: []foldResponse{{Patch: patch}}}
	runtimeA, err := NewRuntime(journal, providerA)
	if err != nil {
		t.Fatal(err)
	}
	first, err := runtimeA.Summon(context.Background(), SummonRequest{
		EngramID:  "evidence-ancestor",
		Reason:    "check evidence level",
		Scene:     "The source and unit checks pass; can the final behavior be announced?",
		Host:      "pi",
		RequestID: "fold-first-summon",
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := runtimeA.Observe(ObserveRequest{
		AccompanimentID: first.Accompaniment.ID,
		Role:            "toolResult",
		Content:         "Chromium exercised the real page: grid layout and both interactions passed.",
		RequestID:       "fold-tool-result",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeA.Release(ReleaseRequest{AccompanimentID: first.Accompaniment.ID, Reason: "host exited before the external result was folded"}); err != nil {
		t.Fatal(err)
	}
	result, err := runtimeA.Outcome(context.Background(), OutcomeRequest{
		AccompanimentID: first.Accompaniment.ID,
		WakeEventID:     first.Wake.WakeEventID,
		SourceKind:      "tool_result",
		SourceEventID:   observation.ObservationEventID,
		RequestID:       "fold-real-outcome",
	})
	if err != nil {
		t.Fatal(err)
	}
	fold := result.SelfFoldEvent.SelfFold
	if fold == nil || fold.Decision != "change" || fold.Posture != patch.Posture {
		t.Fatalf("self-fold = %#v", result.SelfFoldEvent)
	}
	if fold.Actor != "engram:evidence-ancestor" || fold.Authority != "posture/hypothesis" || fold.UserRatified {
		t.Fatalf("self-fold authorship boundary = %#v", fold)
	}
	if got, want := fold.BasisEventIDs, []string{first.Wake.WakeEventID, result.OutcomeEvent.ID, observation.ObservationEventID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("self-fold basis = %v, want %v", got, want)
	}
	if result.OutcomeEvent.Outcome == nil || result.OutcomeEvent.Outcome.SourceRef != "journal-event:"+observation.ObservationEventID || !strings.HasPrefix(result.OutcomeEvent.Outcome.SourceDigest, "sha256:") {
		t.Fatalf("outcome provenance = %#v", result.OutcomeEvent.Outcome)
	}
	after, err := os.ReadFile(engramPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("self-fold rewrote the immutable original Engram file")
	}

	journalB, err := OpenJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	providerB := &foldingTestProvider{WakeSteering: "The current posture now requires evidence-level separation."}
	runtimeB, err := NewRuntime(journalB, providerB)
	if err != nil {
		t.Fatal(err)
	}
	later, err := runtimeB.Summon(context.Background(), SummonRequest{
		EngramID:  "evidence-ancestor",
		Reason:    "later similar claim",
		Scene:     "Unit tests pass, but the closest real execution has not run.",
		Host:      "pi",
		RequestID: "fold-later-summon",
	})
	if err != nil {
		t.Fatal(err)
	}
	if later.Wake.ActiveFoldID != result.SelfFoldEvent.ID {
		t.Fatalf("wake active fold = %q, want %q", later.Wake.ActiveFoldID, result.SelfFoldEvent.ID)
	}
	if len(providerB.WakeCalls) != 1 {
		t.Fatalf("later wake calls = %d", len(providerB.WakeCalls))
	}
	messages := wakeMessages(providerB.WakeCalls[0].Engram, providerB.WakeCalls[0].Events, providerB.WakeCalls[0].Scene)
	if !containsMessageFragment(messages, "user", "[Engram active self-fold boundary") ||
		!containsMessageFragment(messages, "user", "not a user statement") ||
		!containsMessageFragment(messages, "assistant", patch.Posture) {
		t.Fatalf("active self-fold was not loaded with a clear authorship boundary: %#v", messages)
	}
	if got, want := eventKinds(providerB.WakeCalls[0].Events), []string{"opened", "wake_result", "observation", "released", "outcome", "self_fold", "opened"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("later provider events = %v, want %v", got, want)
	}
}

func TestOutcomeRejectsEngramAndMainAgentSelfReportsAsEvidence(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{ID: "source-boundary", Name: "Source Boundary", Messages: []Message{{Role: "user", Content: "Do not turn self-report into fact."}}}); err != nil {
		t.Fatal(err)
	}
	provider := &foldingTestProvider{WakeSteering: "This steering cannot prove its own success."}
	runtime, _ := NewRuntime(journal, provider)
	wake, err := runtime.Summon(context.Background(), SummonRequest{EngramID: "source-boundary", Reason: "test source", Scene: "A claim needs evidence."})
	if err != nil {
		t.Fatal(err)
	}
	assistant, err := runtime.Observe(ObserveRequest{AccompanimentID: wake.Accompaniment.ID, Role: "assistant", Content: "I completed everything successfully."})
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Outcome(context.Background(), OutcomeRequest{
		AccompanimentID: wake.Accompaniment.ID,
		WakeEventID:     wake.Wake.WakeEventID,
		SourceKind:      "tool_result",
		SourceEventID:   assistant.ObservationEventID,
		RequestID:       "reject-assistant-report",
	})
	if err == nil || !strings.Contains(err.Error(), "tool-result role") {
		t.Fatalf("assistant self-report outcome error = %v", err)
	}
	_, err = runtime.Outcome(context.Background(), OutcomeRequest{
		AccompanimentID: wake.Accompaniment.ID,
		WakeEventID:     wake.Wake.WakeEventID,
		SourceKind:      "user_message",
		SourceEventID:   assistant.ObservationEventID,
		RequestID:       "reject-fabricated-user",
	})
	if err == nil || !strings.Contains(err.Error(), "role=user") {
		t.Fatalf("fabricated user outcome error = %v", err)
	}
	_, err = runtime.Outcome(context.Background(), OutcomeRequest{
		AccompanimentID: wake.Accompaniment.ID,
		WakeEventID:     wake.Wake.WakeEventID,
		SourceKind:      "external_observation",
		SourceEventID:   assistant.ObservationEventID,
		SourceRef:       "claimed-external",
		Content:         "not actually external",
		RequestID:       "reject-external-alias",
	})
	if err == nil || !strings.Contains(err.Error(), "cannot claim") {
		t.Fatalf("external alias outcome error = %v", err)
	}
	events, err := journal.Events("source-boundary")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Kind == "outcome" || event.Kind == "self_fold" {
			t.Fatalf("invalid evidence created growth event: %#v", event)
		}
	}
	if len(provider.FoldCalls) != 0 {
		t.Fatalf("invalid evidence reached Engram self-fold: %d calls", len(provider.FoldCalls))
	}
}

func TestPersistedOutcomeRetriesFoldWithoutDuplicatingEvidence(t *testing.T) {
	journal, _ := OpenJournal(t.TempDir())
	if _, err := journal.Create(CreateRequest{ID: "retry-fold", Name: "Retry Fold", Messages: []Message{{Role: "user", Content: "A failed fold must not erase its outcome."}}}); err != nil {
		t.Fatal(err)
	}
	patch := SelfFoldPatch{
		Decision:                     "change",
		UpdateIntent:                 "Retry the self-fold from the already persisted outcome.",
		Posture:                      "Keep the durable outcome even when the first fold provider call failed.",
		RelationToPreviousExperience: "The same request resumes from its existing outcome event.",
		ExpectedDelta:                "Do not duplicate the evidence or the fold on retry.",
	}
	provider := &foldingTestProvider{WakeSteering: "Wait for cited evidence.", Responses: []foldResponse{{Err: errors.New("provider unavailable")}, {Patch: patch}}}
	runtime, _ := NewRuntime(journal, provider)
	wake, err := runtime.Summon(context.Background(), SummonRequest{EngramID: "retry-fold", Reason: "retry test", Scene: "A user later corrects the result."})
	if err != nil {
		t.Fatal(err)
	}
	userObservation, err := runtime.Observe(ObserveRequest{AccompanimentID: wake.Accompaniment.ID, Role: "user", Content: "That intervention was useful only after the tool result arrived."})
	if err != nil {
		t.Fatal(err)
	}
	request := OutcomeRequest{
		AccompanimentID: wake.Accompaniment.ID,
		WakeEventID:     wake.Wake.WakeEventID,
		SourceKind:      "user_message",
		SourceEventID:   userObservation.ObservationEventID,
		RequestID:       "retry-the-same-outcome",
	}
	first, err := runtime.Outcome(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "was persisted") || first.OutcomeEvent.ID == "" {
		t.Fatalf("first failed fold = %#v, %v", first, err)
	}
	second, err := runtime.Outcome(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	third, err := runtime.Outcome(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if second.OutcomeEvent.ID != first.OutcomeEvent.ID || third.SelfFoldEvent.ID != second.SelfFoldEvent.ID {
		t.Fatalf("retry identities changed: first=%#v second=%#v third=%#v", first, second, third)
	}
	if len(provider.FoldCalls) != 2 {
		t.Fatalf("fold provider calls = %d, want 2", len(provider.FoldCalls))
	}
	events, err := journal.Events("retry-fold")
	if err != nil {
		t.Fatal(err)
	}
	outcomes, folds := 0, 0
	for _, event := range events {
		if event.Kind == "outcome" {
			outcomes++
		}
		if event.Kind == "self_fold" {
			folds++
		}
	}
	if outcomes != 1 || folds != 1 {
		t.Fatalf("retry duplicated durable growth: outcomes=%d folds=%d", outcomes, folds)
	}
	conflict := request
	conflict.SourceKind = "external_observation"
	conflict.SourceEventID = ""
	conflict.SourceRef = "different-source"
	conflict.Content = "different-content"
	if _, err := runtime.Outcome(context.Background(), conflict); err == nil || !strings.Contains(err.Error(), "different causal fields") {
		t.Fatalf("conflicting idempotent outcome error = %v", err)
	}
}

func TestNoChangeKeepsCurrentFoldAndRevertRestoresItsParent(t *testing.T) {
	journal, _ := OpenJournal(t.TempDir())
	if _, err := journal.Create(CreateRequest{ID: "reversible-fold", Name: "Reversible Fold", Messages: []Message{{Role: "user", Content: "Growth must remain reversible without deleting history."}}}); err != nil {
		t.Fatal(err)
	}
	change := SelfFoldPatch{
		Decision:                     "change",
		UpdateIntent:                 "Adopt one scoped posture.",
		Posture:                      "Ask for the closest observable evidence before affirming a final claim.",
		RelationToPreviousExperience: "The first tool result exposed an evidence gap.",
		ExpectedDelta:                "A similar claim should remain unverified until observed.",
	}
	noChange := SelfFoldPatch{
		Decision:                     "no_change",
		UpdateIntent:                 "The second result already fits the current posture.",
		RelationToPreviousExperience: "No additional durable change is needed.",
	}
	provider := &foldingTestProvider{WakeSteering: "Require matching evidence.", Responses: []foldResponse{{Patch: change}, {Patch: noChange}}}
	runtime, _ := NewRuntime(journal, provider)
	firstWake, err := runtime.Summon(context.Background(), SummonRequest{EngramID: "reversible-fold", Reason: "first", Scene: "Only source inspection exists."})
	if err != nil {
		t.Fatal(err)
	}
	firstSource, _ := runtime.Observe(ObserveRequest{AccompanimentID: firstWake.Accompaniment.ID, Role: "toolResult", Content: "The browser run was still missing."})
	firstOutcome, err := runtime.Outcome(context.Background(), OutcomeRequest{AccompanimentID: firstWake.Accompaniment.ID, WakeEventID: firstWake.Wake.WakeEventID, SourceKind: "tool_result", SourceEventID: firstSource.ObservationEventID, RequestID: "first-fold-outcome"})
	if err != nil {
		t.Fatal(err)
	}
	secondWake, err := runtime.Wake(context.Background(), WakeRequest{AccompanimentID: firstWake.Accompaniment.ID, Scene: "The closest browser run now exists."})
	if err != nil {
		t.Fatal(err)
	}
	secondSource, _ := runtime.Observe(ObserveRequest{AccompanimentID: firstWake.Accompaniment.ID, Role: "toolResult", Content: "The browser run passed the claimed interaction."})
	secondOutcome, err := runtime.Outcome(context.Background(), OutcomeRequest{AccompanimentID: firstWake.Accompaniment.ID, WakeEventID: secondWake.WakeEventID, SourceKind: "tool_result", SourceEventID: secondSource.ObservationEventID, RequestID: "second-fold-outcome"})
	if err != nil {
		t.Fatal(err)
	}
	if secondOutcome.SelfFoldEvent.SelfFold.Decision != "no_change" || secondOutcome.ActiveFoldID != firstOutcome.SelfFoldEvent.ID {
		t.Fatalf("no_change moved current posture: %#v", secondOutcome)
	}
	status, err := runtime.FoldStatus(FoldStatusRequest{EngramID: "reversible-fold"})
	if err != nil {
		t.Fatal(err)
	}
	if status.ActiveFoldID != firstOutcome.SelfFoldEvent.ID || len(status.History) != 2 {
		t.Fatalf("fold status before revert = %#v", status)
	}
	reverted, err := runtime.RevertFold(FoldRevertRequest{EngramID: "reversible-fold", FoldEventID: firstOutcome.SelfFoldEvent.ID, Reason: "return to the unfurled original posture", RequestID: "revert-first-fold"})
	if err != nil {
		t.Fatal(err)
	}
	if reverted.ActiveFoldID != "" || reverted.ActiveFold != nil || len(reverted.History) != 3 {
		t.Fatalf("fold status after revert = %#v", reverted)
	}
	again, err := runtime.RevertFold(FoldRevertRequest{EngramID: "reversible-fold", FoldEventID: firstOutcome.SelfFoldEvent.ID, Reason: "return to the unfurled original posture", RequestID: "revert-first-fold"})
	if err != nil || len(again.History) != 3 {
		t.Fatalf("idempotent revert = %#v, %v", again, err)
	}
	if _, err := runtime.RevertFold(FoldRevertRequest{EngramID: "reversible-fold", FoldEventID: firstOutcome.SelfFoldEvent.ID, Reason: "a contradictory replacement reason", RequestID: "revert-first-fold"}); err == nil || !strings.Contains(err.Error(), "different fold or reason") {
		t.Fatalf("conflicting idempotent revert error = %v", err)
	}
	if _, err := runtime.Release(ReleaseRequest{AccompanimentID: firstWake.Accompaniment.ID, Reason: "revert test complete"}); err != nil {
		t.Fatal(err)
	}
	providerAfterRevert := &foldingTestProvider{WakeSteering: "Continue from original history without the reverted posture."}
	runtimeAfterRevert, _ := NewRuntime(journal, providerAfterRevert)
	later, err := runtimeAfterRevert.Summon(context.Background(), SummonRequest{EngramID: "reversible-fold", Reason: "after revert", Scene: "A new claim appears."})
	if err != nil {
		t.Fatal(err)
	}
	if later.Wake.ActiveFoldID != "" {
		t.Fatalf("reverted fold remained active: %q", later.Wake.ActiveFoldID)
	}
	messages := wakeMessages(providerAfterRevert.WakeCalls[0].Engram, providerAfterRevert.WakeCalls[0].Events, providerAfterRevert.WakeCalls[0].Scene)
	if containsMessageFragment(messages, "user", "[Engram active self-fold boundary") {
		t.Fatalf("reverted fold was still loaded as the current posture: %#v", messages)
	}
	if !containsMessageFragment(messages, "user", "[Engram historical self-fold; event_id="+firstOutcome.SelfFoldEvent.ID) ||
		!containsMessageFragment(messages, "assistant", change.Posture) ||
		!containsMessageFragment(messages, "user", "[Self-fold correction;") ||
		!containsMessageFragment(messages, "user", "return to the unfurled original posture") {
		t.Fatalf("reverted fold and its correction were erased from Engram experience: %#v", messages)
	}
}

func containsMessageFragment(messages []providerMessage, role, fragment string) bool {
	for _, message := range messages {
		if message.Role == role && strings.Contains(message.Content, fragment) {
			return true
		}
	}
	return false
}
