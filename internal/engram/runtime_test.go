package engram

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"strings"
	"testing"
)

type silentTestProvider struct{}

func (silentTestProvider) Wake(context.Context, Engram, []JournalEvent, string) (Decision, error) {
	return Decision{State: "silent", Reason: "not relevant"}, nil
}

func (silentTestProvider) Fold(context.Context, Engram, []JournalEvent, JournalEvent) (SelfFoldPatch, error) {
	return SelfFoldPatch{Decision: "no_change", UpdateIntent: "No durable update.", RelationToPreviousExperience: "The cited outcome does not change this test Engram."}, nil
}

type recordedWake struct {
	Engram Engram
	Events []JournalEvent
	Scene  string
}

type recordingTestProvider struct {
	Steering string
	Calls    []recordedWake
}

func (provider *recordingTestProvider) Wake(_ context.Context, value Engram, events []JournalEvent, scene string) (Decision, error) {
	value.Messages = append([]Message(nil), value.Messages...)
	provider.Calls = append(provider.Calls, recordedWake{
		Engram: value,
		Events: append([]JournalEvent(nil), events...),
		Scene:  scene,
	})
	return Decision{State: "active", Reason: "recorded fresh request", Steering: provider.Steering}, nil
}

func (provider *recordingTestProvider) Fold(_ context.Context, _ Engram, _ []JournalEvent, outcome JournalEvent) (SelfFoldPatch, error) {
	return SelfFoldPatch{
		Decision:                     "change",
		UpdateIntent:                 "Record the cited outcome in this test posture.",
		Posture:                      "The test Engram carries outcome " + outcome.ID + " as scoped evidence.",
		RelationToPreviousExperience: "The outcome follows a recorded wake.",
		ExpectedDelta:                "A later wake can identify the cited result.",
	}, nil
}

func TestEngramAccompaniesMultipleRounds(t *testing.T) {
	journal, err := OpenJournal(filepath.Join(t.TempDir(), "journal"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := journal.Create(CreateRequest{ID: "ancestor", Name: "Ancestor", Statement: "Protect the original design.", Messages: []Message{{Role: "user", Content: "A past Agent session should return as a participant."}, {Role: "assistant", Content: "Its original context is its historical body."}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Messages) != 2 {
		t.Fatalf("messages = %d", len(value.Messages))
	}
	runtime, _ := NewRuntime(journal, RuleProvider{})
	first, err := runtime.Summon(context.Background(), SummonRequest{EngramID: "ancestor", Reason: "review", Scene: "Has the design drifted?", Host: "codex", HostSession: "one", RequestID: "summon-one"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Wake.State != "active" || first.Wake.Steering != "Protect the original design." {
		t.Fatalf("summon = %#v", first)
	}
	if first.Wake.Attribution.EngramID != "ancestor" || first.Wake.Attribution.Name != "Ancestor" || first.Wake.Attribution.Statement != "Protect the original design." {
		t.Fatalf("attribution = %#v", first.Wake.Attribution)
	}
	if first.Wake.Attribution.AccompanimentID != first.Accompaniment.ID {
		t.Fatalf("attribution accompaniment = %q, want %q", first.Wake.Attribution.AccompanimentID, first.Accompaniment.ID)
	}
	id := first.Accompaniment.ID
	if _, err := runtime.Observe(ObserveRequest{AccompanimentID: id, Role: "assistant", Content: "I kept the design."}); err != nil {
		t.Fatal(err)
	}
	second, err := runtime.Wake(context.Background(), WakeRequest{AccompanimentID: id, Scene: "Check the same Engram again."})
	if err != nil {
		t.Fatal(err)
	}
	if second.Accompaniment.ID != id {
		t.Fatal("wake opened a different accompaniment")
	}
	if second.Attribution != first.Wake.Attribution {
		t.Fatalf("attribution changed across accompaniment: %#v != %#v", second.Attribution, first.Wake.Attribution)
	}
	released, err := runtime.Release(ReleaseRequest{AccompanimentID: id, Reason: "done"})
	if err != nil {
		t.Fatal(err)
	}
	if released.Status != "sleeping" {
		t.Fatalf("release = %#v", released)
	}
	if _, err := runtime.Wake(context.Background(), WakeRequest{AccompanimentID: id, Scene: "too late"}); err == nil {
		t.Fatal("sleeping accompaniment woke")
	}
}

func TestEngramResummonsAcrossRuntimeRestartWithContinuousContext(t *testing.T) {
	root := t.TempDir()
	original := []Message{
		{ID: "origin-1", Role: "system", Content: "Keep the historical body complete."},
		{ID: "origin-2", Role: "user", Content: "A provider-owned thread is not the continuity source."},
		{ID: "origin-3", Role: "assistant", Content: "The runtime must resubmit the same context."},
	}
	journalA, err := OpenJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journalA.Create(CreateRequest{ID: "restart-ancestor", Name: "Restart Ancestor", Messages: original}); err != nil {
		t.Fatal(err)
	}
	providerA := &recordingTestProvider{Steering: "Keep the exact Journal through the restart."}
	runtimeA, err := NewRuntime(journalA, providerA)
	if err != nil {
		t.Fatal(err)
	}
	first, err := runtimeA.Summon(context.Background(), SummonRequest{
		EngramID: "restart-ancestor",
		Reason:   "first accompaniment",
		Scene:    "The host is about to restart.",
		Host:     "pi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeA.Observe(ObserveRequest{
		AccompanimentID: first.Accompaniment.ID,
		Role:            "assistant",
		Content:         "The host persisted the exact session log.",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeA.Release(ReleaseRequest{AccompanimentID: first.Accompaniment.ID, Reason: "host process stopped"}); err != nil {
		t.Fatal(err)
	}

	journalB, err := OpenJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	providerB := &recordingTestProvider{Steering: "The prior result is still part of my history."}
	runtimeB, err := NewRuntime(journalB, providerB)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtimeB.Summon(context.Background(), SummonRequest{
		EngramID: "restart-ancestor",
		Reason:   "later resummon",
		Scene:    "The host has started again with a new provider request.",
		Host:     "pi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Accompaniment.ID == first.Accompaniment.ID {
		t.Fatal("resummon reused the released accompaniment")
	}
	if len(providerB.Calls) != 1 {
		t.Fatalf("fresh provider calls = %d", len(providerB.Calls))
	}
	call := providerB.Calls[0]
	if !reflect.DeepEqual(call.Engram.Messages, original) {
		t.Fatalf("original context changed across restart:\n got %#v\nwant %#v", call.Engram.Messages, original)
	}
	if got, want := eventKinds(call.Events), []string{"opened", "wake_result", "observation", "released", "opened"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resummon provider events = %v, want %v", got, want)
	}
	messages := wakeMessages(call.Engram, call.Events, call.Scene)
	wantMessages := []providerMessage{
		{Role: "system", Content: original[0].Content},
		{Role: "user", Content: original[1].Content},
		{Role: "assistant", Content: original[2].Content},
		{Role: "user", Content: wakeBoundary("first accompaniment")},
		activeThreadSlice("The host is about to restart."),
		{Role: "assistant", Content: "Keep the exact Journal through the restart."},
		observedThreadMessage("assistant", "The host persisted the exact session log."),
		releasedAccompanimentMessage(call.Events[3]),
		{Role: "user", Content: wakeBoundary("later resummon")},
		activeThreadSlice("The host has started again with a new provider request."),
	}
	if !reflect.DeepEqual(messages, wantMessages) {
		t.Fatalf("reconstructed provider context:\n got %#v\nwant %#v", messages, wantMessages)
	}
	events, err := journalB.Events("restart-ancestor")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := eventKinds(events), []string{"opened", "wake_result", "observation", "released", "opened", "wake_result"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("durable resummon history = %v, want %v", got, want)
	}
}

func TestWakeAndObservationRequestIDsReplayWithoutDuplicateCausalEvents(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{
		ID:       "idempotent-events",
		Name:     "Idempotent Events",
		Messages: []Message{{Role: "user", Content: "A retry must not create a second causal event."}},
	}); err != nil {
		t.Fatal(err)
	}
	provider := &recordingTestProvider{Steering: "Keep one durable event per stable request."}
	runtime, err := NewRuntime(journal, provider)
	if err != nil {
		t.Fatal(err)
	}
	summoned, err := runtime.Summon(context.Background(), SummonRequest{
		EngramID: "idempotent-events",
		Reason:   "open retry test",
		Scene:    "initial",
	})
	if err != nil {
		t.Fatal(err)
	}
	wakeRequest := WakeRequest{
		AccompanimentID: summoned.Accompaniment.ID,
		Scene:           "stable later scene",
		HostTurnID:      "turn-retry",
		RequestID:       "shared-retry-id",
	}
	firstWake, err := runtime.Wake(context.Background(), wakeRequest)
	if err != nil {
		t.Fatal(err)
	}
	secondWake, err := runtime.Wake(context.Background(), wakeRequest)
	if err != nil {
		t.Fatal(err)
	}
	if firstWake.WakeEventID != secondWake.WakeEventID || len(provider.Calls) != 2 {
		t.Fatalf("wake retry duplicated provider work: first=%q second=%q calls=%d", firstWake.WakeEventID, secondWake.WakeEventID, len(provider.Calls))
	}
	conflictingWake := wakeRequest
	conflictingWake.Scene = "different scene"
	if _, err := runtime.Wake(context.Background(), conflictingWake); err == nil || !strings.Contains(err.Error(), "different scene") {
		t.Fatalf("conflicting wake retry error = %v", err)
	}

	observeRequest := ObserveRequest{
		AccompanimentID: summoned.Accompaniment.ID,
		Role:            "toolResult",
		Content:         "exact durable result",
		HostTurnID:      "turn-retry",
		RequestID:       "shared-retry-id",
	}
	firstObservation, err := runtime.Observe(observeRequest)
	if err != nil {
		t.Fatal(err)
	}
	if firstObservation.ObservationEventID == firstWake.WakeEventID {
		t.Fatal("wake and observation reused one event id for the same request_id")
	}
	if _, err := runtime.Release(ReleaseRequest{AccompanimentID: summoned.Accompaniment.ID, Reason: "retry after response loss"}); err != nil {
		t.Fatal(err)
	}
	secondObservation, err := runtime.Observe(observeRequest)
	if err != nil {
		t.Fatal(err)
	}
	if secondObservation.ObservationEventID != firstObservation.ObservationEventID {
		t.Fatalf("observation retry id changed: first=%q second=%q", firstObservation.ObservationEventID, secondObservation.ObservationEventID)
	}
	conflictingObservation := observeRequest
	conflictingObservation.Content = "different result"
	if _, err := runtime.Observe(conflictingObservation); err == nil || !strings.Contains(err.Error(), "different role, content") {
		t.Fatalf("conflicting observation retry error = %v", err)
	}
	events, err := journal.Events("idempotent-events")
	if err != nil {
		t.Fatal(err)
	}
	wakes, observations := 0, 0
	for _, event := range events {
		if event.Kind == "wake_result" && event.RequestID == wakeRequest.RequestID {
			wakes++
		}
		if event.Kind == "observation" && event.RequestID == observeRequest.RequestID {
			observations++
		}
	}
	if wakes != 1 || observations != 1 {
		t.Fatalf("stable retries wrote duplicate causal events: wakes=%d observations=%d", wakes, observations)
	}
}

func TestCurrentRuntimeResummonsARealV013Journal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "legacy")
	copyFixtureTree(t, filepath.Join("testdata", "v0.1.3"), root)
	journal, err := OpenJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	provider := &recordingTestProvider{Steering: "The current runtime retained the legacy experience."}
	runtime, err := NewRuntime(journal, provider)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Summon(context.Background(), SummonRequest{
		EngramID: "legacy-resummon",
		Reason:   "verify upgrade continuity",
		Scene:    "The v0.2+ runtime reopened this v0.1.3 Journal.",
		Host:     "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Accompaniment.ID == "acc-5443d830f0e7c17aefe9c95f6a325de2" {
		t.Fatal("current runtime reused the v0.1.3 sleeping accompaniment")
	}
	if len(provider.Calls) != 1 {
		t.Fatalf("fresh provider calls = %d", len(provider.Calls))
	}
	call := provider.Calls[0]
	if len(call.Engram.Messages) != 3 || call.Engram.Messages[0].ID != "origin-1" || call.Engram.Messages[2].Content != "A fresh request is acceptable only when the same historical context is resubmitted." {
		t.Fatalf("legacy original context was not recovered exactly: %#v", call.Engram.Messages)
	}
	if got, want := eventKinds(call.Events), []string{"opened", "wake_result", "observation", "released", "opened"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy resummon provider events = %v, want %v", got, want)
	}
	providerMessages := wakeMessages(call.Engram, call.Events, call.Scene)
	if !containsProviderMessage(providerMessages, "assistant", "Preserve continuity across runtime upgrades.") ||
		!containsProviderMessage(providerMessages, "user", "[Observed active-thread message; role=assistant]\nThe upgrade preserved the Journal on disk.") {
		t.Fatalf("legacy accompaniment history missing from fresh provider request: %#v", providerMessages)
	}
}

func eventKinds(events []JournalEvent) []string {
	kinds := make([]string, len(events))
	for i, event := range events {
		kinds[i] = event.Kind
	}
	return kinds
}

func containsProviderMessage(messages []providerMessage, role, content string) bool {
	for _, message := range messages {
		if message.Role == role && message.Content == content {
			return true
		}
	}
	return false
}

func copyFixtureTree(t *testing.T, source, target string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, content, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOriginalContextIsStoredExactly(t *testing.T) {
	journal, _ := OpenJournal(t.TempDir())
	original := "中文原始上下文\nwith exact spacing"
	_, err := journal.Create(CreateRequest{ID: "raw", Name: "Raw", Messages: []Message{{ID: "m1", Role: "user", Content: original}}})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := journal.Load("raw")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Messages[0].Content != original {
		t.Fatalf("content = %q", loaded.Messages[0].Content)
	}
}

func TestSilentWakeStillIdentifiesTheEngramWithoutFabricatingSpeech(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{
		ID:       "quiet-ancestor",
		Name:     "Quiet Ancestor",
		Messages: []Message{{Role: "user", Content: "Silence can be a judgment."}},
	}); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntime(journal, silentTestProvider{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Summon(context.Background(), SummonRequest{
		EngramID: "quiet-ancestor",
		Reason:   "check relevance",
		Scene:    "An unrelated task",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Wake.State != "silent" || result.Wake.Steering != "" || AttributedSteeringText(result.Wake) != "" {
		t.Fatalf("silent wake fabricated speech: %#v", result.Wake)
	}
	if result.Wake.Attribution.EngramID != "quiet-ancestor" || result.Wake.Attribution.Name != "Quiet Ancestor" || result.Wake.Attribution.AccompanimentID != result.Accompaniment.ID {
		t.Fatalf("silent attribution = %#v", result.Wake.Attribution)
	}
}

func TestGuardianObserveFailsWithoutDurableAccompanimentIndex(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntime(journal, RuleProvider{})
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.GuardianObserve("pi", "missing-session", "missing-engram", "assistant", "must not be acknowledged")
	if err == nil || !strings.Contains(err.Error(), "index not found") {
		t.Fatalf("missing guardian index error = %v", err)
	}
}

func TestGuardianWakeReportsIndexPersistenceFailureAndSleepsOrphan(t *testing.T) {
	root := t.TempDir()
	journal, err := OpenJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{
		ID:       "guardian-index-failure",
		Name:     "Guardian Index Failure",
		Messages: []Message{{Role: "user", Content: "Do not claim continuity without a durable address."}},
	}); err != nil {
		t.Fatal(err)
	}
	guardiansPath := filepath.Join(root, "guardians")
	if goruntime.GOOS == "windows" {
		if err := os.Remove(guardiansPath); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(guardiansPath, []byte("blocks index directory creation"), 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := os.Chmod(guardiansPath, 0o500); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := os.Chmod(guardiansPath, 0o700); err != nil {
				t.Errorf("restore guardian directory permissions: %v", err)
			}
		}()
	}
	runtime, err := NewRuntime(journal, RuleProvider{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.GuardianWake(context.Background(), "guardian-index-failure", "pi", "session", root, "turn", "current scene", 1800)
	if err == nil || !strings.Contains(err.Error(), "persist guardian accompaniment index") {
		t.Fatalf("guardian wake index error = %v", err)
	}
	events, err := journal.Events("guardian-index-failure")
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if last.Kind != "released" || last.Reason != "guardian index persistence failed" {
		t.Fatalf("orphan accompaniment was left active: %#v", last)
	}
}

func TestGuardianReleaseCleansAnAlreadySleepingIndex(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{
		ID:       "guardian-release",
		Name:     "Guardian Release",
		Messages: []Message{{Role: "user", Content: "Release should be bounded and retryable."}},
	}); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntime(journal, RuleProvider{})
	if err != nil {
		t.Fatal(err)
	}
	wake, err := runtime.GuardianWake(context.Background(), "guardian-release", "pi", "release-session", "", "turn", "current scene", 1800)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Release(ReleaseRequest{AccompanimentID: wake.Accompaniment.ID, Reason: "released directly"}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.GuardianRelease("pi", "release-session", "guardian-release"); err != nil {
		t.Fatal(err)
	}
	var index map[string]string
	found, err := journal.ReadIndex("guardians", guardianKey("pi", "release-session", "guardian-release"), &index)
	if err != nil || found {
		t.Fatalf("sleeping guardian index remained: %#v, %v, %v", index, found, err)
	}
}

func TestTakePendingUpgradesV012AttributionBeforeDelivery(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{
		ID:        "legacy-ancestor",
		Name:      "Legacy Ancestor",
		Statement: "Keep old pending speech attributable.",
		Messages:  []Message{{Role: "user", Content: "Original context"}},
	}); err != nil {
		t.Fatal(err)
	}
	legacy := PendingSteering{
		ProtocolVersion: ProtocolVersion,
		AccompanimentID: "acc-v012",
		EngramID:        "legacy-ancestor",
		Host:            HostRef{Kind: "cursor", SessionID: "legacy-session"},
		State:           "active",
		Steering:        "old pending steering",
	}
	if err := withJournalLockErr(journal, func() error {
		return journal.writeIndexLocked("pending", guardianKey("cursor", "legacy-session", "legacy-ancestor"), legacy)
	}); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntime(journal, RuleProvider{})
	if err != nil {
		t.Fatal(err)
	}
	upgraded, found, err := runtime.TakePending("legacy-ancestor", "cursor", "legacy-session")
	if err != nil || !found {
		t.Fatalf("upgraded pending = %#v, %v, %v", upgraded, found, err)
	}
	if upgraded.Attribution.EngramID != "legacy-ancestor" || upgraded.Attribution.Name != "Legacy Ancestor" || upgraded.Attribution.Statement != "Keep old pending speech attributable." || upgraded.Attribution.AccompanimentID != "acc-v012" {
		t.Fatalf("upgraded attribution = %#v", upgraded.Attribution)
	}
}
