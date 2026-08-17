package engram

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJournalFormatGuardMakesOldReadersFailClosed(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{
		ID:       "downgrade-guard",
		Name:     "Downgrade Guard",
		Messages: []Message{{Role: "user", Content: "Never let an older runtime silently omit newer experience."}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Append("downgrade-guard", JournalEvent{Kind: "observation", Role: "assistant", Content: "current event"}); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(journal.engramDir("downgrade-guard"), "journal.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 || lines[0] != `"`+journalFormatMarker+`"` {
		t.Fatalf("Journal guard lines = %q", lines)
	}
	var legacyEvent JournalEvent
	if err := json.Unmarshal([]byte(lines[0]), &legacyEvent); err == nil {
		t.Fatal("a v0.1/v0.2-style JournalEvent decoder accepted the v0.3 downgrade guard")
	}
	events, err := journal.Events("downgrade-guard")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Content != "current event" {
		t.Fatalf("current reader lost guarded events: %#v", events)
	}
}

func TestAppendWritesDurableGuardEvenWhenAStaleSidecarExists(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{
		ID:       "stale-sidecar",
		Name:     "Stale Sidecar",
		Messages: []Message{{Role: "user", Content: "The Journal itself is the durable truth."}},
	}); err != nil {
		t.Fatal(err)
	}
	dir := journal.engramDir("stale-sidecar")
	legacy, err := json.Marshal(JournalEvent{
		ProtocolVersion: ProtocolVersion,
		ID:              "evt-legacy",
		Kind:            "observation",
		OccurredAt:      nowUTC(),
		Role:            "assistant",
		Content:         "legacy snapshot without a format guard",
	})
	if err != nil {
		t.Fatal(err)
	}
	legacy = append(legacy, '\n')
	if err := os.WriteFile(filepath.Join(dir, "journal.jsonl"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	// This is the cache filename used by an earlier release-candidate design.
	// Its presence must never let an append skip the guard in the Journal.
	if err := os.WriteFile(filepath.Join(dir, ".journal-format-v0.3"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := journal.Append("stale-sidecar", JournalEvent{Kind: "observation", Role: "toolResult", Content: "new guarded event"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 3 || lines[1] != `"`+journalFormatMarker+`"` {
		t.Fatalf("append trusted stale sidecar instead of writing durable guard: %q", lines)
	}
	events, err := journal.Events("stale-sidecar")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Content != "new guarded event" {
		t.Fatalf("guarded append did not remain readable: %#v", events)
	}
}

func TestAppendPreservesReadableLegacyJournalWithoutTerminalNewline(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{
		ID:       "legacy-no-newline",
		Name:     "Legacy Without Newline",
		Messages: []Message{{Role: "user", Content: "A readable history must remain readable after append."}},
	}); err != nil {
		t.Fatal(err)
	}
	legacy, err := json.Marshal(JournalEvent{
		ProtocolVersion: ProtocolVersion,
		ID:              "evt-legacy-no-newline",
		Kind:            "observation",
		OccurredAt:      nowUTC(),
		Role:            "assistant",
		Content:         "complete legacy JSON without a terminal newline",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(journal.engramDir("legacy-no-newline"), "journal.jsonl")
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if events, err := journal.Events("legacy-no-newline"); err != nil || len(events) != 1 {
		t.Fatalf("legacy Journal should be readable before append: events=%#v err=%v", events, err)
	}
	if err := journal.Append("legacy-no-newline", JournalEvent{Kind: "observation", Role: "toolResult", Content: "new guarded event"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 3 || lines[1] != `"`+journalFormatMarker+`"` {
		t.Fatalf("append did not restore the JSONL record boundary: %q", lines)
	}
	events, err := journal.Events("legacy-no-newline")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Content != "new guarded event" {
		t.Fatalf("append damaged the readable legacy Journal: %#v", events)
	}
}

func TestV03GrowthEventsRequireDowngradeGuard(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{
		ID:       "unguarded-growth",
		Name:     "Unguarded Growth",
		Messages: []Message{{Role: "user", Content: "Refuse unsafe mixed Journal formats."}},
	}); err != nil {
		t.Fatal(err)
	}
	line, err := json.Marshal(JournalEvent{
		ProtocolVersion: ProtocolVersion,
		ID:              "evt-unguarded-outcome",
		Kind:            "outcome",
		OccurredAt:      nowUTC(),
		Outcome:         &OutcomeRecord{WakeEventID: "evt-wake", SourceKind: "external_observation", SourceRef: "fixture", SourceDigest: "sha256:fixture"},
	})
	if err != nil {
		t.Fatal(err)
	}
	line = append(line, '\n')
	path := filepath.Join(journal.engramDir("unguarded-growth"), "journal.jsonl")
	if err := os.WriteFile(path, line, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Events("unguarded-growth"); err == nil || !strings.Contains(err.Error(), "downgrade guard") {
		t.Fatalf("unguarded v0.3 event error = %v", err)
	}
}

func TestEventsRejectUnknownKindInsteadOfIgnoringExperience(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{
		ID:       "unknown-kind",
		Name:     "Unknown Kind",
		Messages: []Message{{Role: "user", Content: "Unknown experience must be visible as incompatibility."}},
	}); err != nil {
		t.Fatal(err)
	}
	marker, err := json.Marshal(journalFormatMarker)
	if err != nil {
		t.Fatal(err)
	}
	event, err := json.Marshal(JournalEvent{ProtocolVersion: ProtocolVersion, ID: "evt-future", Kind: "future_experience", OccurredAt: nowUTC()})
	if err != nil {
		t.Fatal(err)
	}
	raw := append(marker, '\n')
	raw = append(raw, event...)
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(journal.engramDir("unknown-kind"), "journal.jsonl"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Events("unknown-kind"); err == nil || !strings.Contains(err.Error(), "unsupported event kind") {
		t.Fatalf("unknown Journal event error = %v", err)
	}
}

func TestEventsRejectsOversizedJournalInsteadOfReturningPartialHistory(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{
		ID:        "oversized",
		Name:      "Oversized",
		Statement: "Never hide missing history.",
		Messages:  []Message{{Role: "user", Content: "original"}},
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(journal.engramDir("oversized"), "journal.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxJournalBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = journal.Events("oversized")
	if err == nil || !strings.Contains(err.Error(), "refusing to wake with incomplete history") {
		t.Fatalf("Events error = %v", err)
	}
}

func TestLoadRejectsUnknownEngramProtocolInsteadOfGuessing(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{
		ID:       "future-engram",
		Name:     "Future Engram",
		Messages: []Message{{Role: "user", Content: "Do not guess across an unknown storage protocol."}},
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(journal.engramDir("future-engram"), "engram.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), ProtocolVersion, "engram/future", 1))
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Load("future-engram"); err == nil || !strings.Contains(err.Error(), "unsupported protocol_version") {
		t.Fatalf("Load error = %v", err)
	}
}

func TestEventsRejectUnknownJournalProtocolInsteadOfSkippingHistory(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{
		ID:       "future-event",
		Name:     "Future Event",
		Messages: []Message{{Role: "user", Content: "Every event must remain interpretable."}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Append("future-event", JournalEvent{Kind: "observation", Role: "assistant", Content: "known event"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(journal.engramDir("future-event"), "journal.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), ProtocolVersion, "engram/future", 1))
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Events("future-event"); err == nil || !strings.Contains(err.Error(), "unsupported protocol_version") {
		t.Fatalf("Events error = %v", err)
	}
}

func TestJournalLockCoordinatesSeparateProcesses(t *testing.T) {
	if os.Getenv("ENGRAM_LOCK_HELPER") == "1" {
		runJournalLockHelper(t)
		return
	}

	root := t.TempDir()
	journal, err := OpenJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Create(CreateRequest{
		ID:        "locked",
		Name:      "Locked",
		Statement: "Serialize durable history.",
		Messages:  []Message{{Role: "user", Content: "original"}},
	}); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(root, "helper-ready")
	done := filepath.Join(root, "helper-done")
	command := exec.Command(os.Args[0], "-test.run=^TestJournalLockCoordinatesSeparateProcesses$")
	command.Env = append(os.Environ(),
		"ENGRAM_LOCK_HELPER=1",
		"ENGRAM_LOCK_ROOT="+root,
		"ENGRAM_LOCK_READY="+ready,
		"ENGRAM_LOCK_DONE="+done,
	)

	if err := withJournalLockErr(journal, func() error {
		if err := command.Start(); err != nil {
			return err
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(ready); err == nil {
				break
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if time.Now().After(deadline) {
				return errors.New("helper did not reach the locked append")
			}
			time.Sleep(10 * time.Millisecond)
		}
		time.Sleep(100 * time.Millisecond)
		if _, err := os.Stat(done); err == nil {
			return errors.New("helper crossed the process lock before release")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}); err != nil {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(done); err != nil {
		t.Fatalf("helper did not finish after lock release: %v", err)
	}
	events, err := journal.Events("locked")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Content != "cross-process append" {
		t.Fatalf("events = %#v", events)
	}
}

func runJournalLockHelper(t *testing.T) {
	root := os.Getenv("ENGRAM_LOCK_ROOT")
	journal, err := OpenJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("ENGRAM_LOCK_READY"), []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := journal.Append("locked", JournalEvent{Kind: "observation", Role: "assistant", Content: "cross-process append"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("ENGRAM_LOCK_DONE"), []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
}
