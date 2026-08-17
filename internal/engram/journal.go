package engram

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const maxJournalBytes int64 = 128 * 1024 * 1024

const journalFormatMarker = "engram-journal/v0.3"

type Journal struct {
	Root string
	mu   sync.Mutex
}

func OpenJournal(root string) (*Journal, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("Engram data directory is required")
	}
	for _, dir := range []string{"engrams", "accompaniments", "guardians", "pending"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			return nil, err
		}
	}
	return &Journal{Root: root}, nil
}

func (j *Journal) Create(request CreateRequest) (Engram, error) {
	return withJournalLock(j, func() (Engram, error) {
		return j.createLocked(request)
	})
}

func (j *Journal) createLocked(request CreateRequest) (Engram, error) {
	if err := validateID(request.ID); err != nil {
		return Engram{}, fmt.Errorf("invalid Engram id: %w", err)
	}
	if err := required("Engram name", request.Name, 256); err != nil {
		return Engram{}, err
	}
	if len(request.Statement) > 16384 {
		return Engram{}, errors.New("Engram catalog note exceeds 16384 bytes")
	}
	if len(request.Messages) == 0 {
		return Engram{}, errors.New("at least one original message is required")
	}
	seen := map[string]bool{}
	for i := range request.Messages {
		m := &request.Messages[i]
		if err := required("message role", m.Role, 80); err != nil {
			return Engram{}, fmt.Errorf("message %d: %w", i+1, err)
		}
		if err := required("message content", m.Content, 4*1024*1024); err != nil {
			return Engram{}, fmt.Errorf("message %d: %w", i+1, err)
		}
		if m.ID == "" {
			m.ID = fmt.Sprintf("message-%06d", i+1)
		}
		if seen[m.ID] {
			return Engram{}, fmt.Errorf("message %d repeats id %q", i+1, m.ID)
		}
		seen[m.ID] = true
	}
	value := Engram{ProtocolVersion: ProtocolVersion, ID: request.ID, Name: request.Name, Statement: request.Statement, Messages: request.Messages, CreatedAt: nowUTC()}
	dir := j.engramDir(request.ID)
	if err := os.Mkdir(dir, 0o700); err != nil {
		if os.IsExist(err) {
			return Engram{}, fmt.Errorf("Engram %q already exists", request.ID)
		}
		return Engram{}, err
	}
	if err := writeJSONExclusive(filepath.Join(dir, "engram.json"), value); err != nil {
		return Engram{}, err
	}
	return value, nil
}

func (j *Journal) Load(id string) (Engram, error) {
	return withJournalLock(j, func() (Engram, error) {
		return j.loadLocked(id)
	})
}

func (j *Journal) loadLocked(id string) (Engram, error) {
	if err := validateID(id); err != nil {
		return Engram{}, err
	}
	var value Engram
	if err := readJSON(filepath.Join(j.engramDir(id), "engram.json"), &value); err != nil {
		return Engram{}, fmt.Errorf("load Engram %q: %w", id, err)
	}
	if value.ProtocolVersion != ProtocolVersion {
		return Engram{}, fmt.Errorf("load Engram %q: unsupported protocol_version %q (runtime supports %q)", id, value.ProtocolVersion, ProtocolVersion)
	}
	return value, nil
}

func (j *Journal) List() ([]Summary, error) {
	return withJournalLock(j, func() ([]Summary, error) {
		return j.listLocked()
	})
}

func (j *Journal) listLocked() ([]Summary, error) {
	entries, err := os.ReadDir(filepath.Join(j.Root, "engrams"))
	if err != nil {
		return nil, err
	}
	items := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		value, err := j.loadLocked(entry.Name())
		if err != nil {
			return nil, err
		}
		items = append(items, Summary{ProtocolVersion: ProtocolVersion, ID: value.ID, Name: value.Name, CreatedAt: value.CreatedAt})
	}
	sort.Slice(items, func(i, k int) bool { return items[i].ID < items[k].ID })
	return items, nil
}

func (j *Journal) Append(engramID string, event JournalEvent) error {
	return withJournalLockErr(j, func() error {
		return j.appendLocked(engramID, event)
	})
}

func (j *Journal) appendLocked(engramID string, event JournalEvent) error {
	event.ProtocolVersion = ProtocolVersion
	if !knownJournalEventKind(event.Kind) {
		return fmt.Errorf("unsupported Journal event kind %q", event.Kind)
	}
	if event.ID == "" {
		event.ID = randomID("evt")
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = nowUTC()
	}
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	marker, err := json.Marshal(journalFormatMarker)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(j.engramDir(engramID), "journal.jsonl"), os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	payload := make([]byte, 0, len(marker)+len(line)+3)
	if info.Size() > 0 {
		var last [1]byte
		if _, err := file.ReadAt(last[:], info.Size()-1); err != nil {
			return fmt.Errorf("inspect Journal boundary: %w", err)
		}
		if last[0] != '\n' {
			payload = append(payload, '\n')
		}
	}
	payload = append(payload, marker...)
	payload = append(payload, '\n')
	payload = append(payload, line...)
	payload = append(payload, '\n')
	if _, err := file.Write(payload); err != nil {
		return err
	}
	return file.Sync()
}

func (j *Journal) Events(engramID string) ([]JournalEvent, error) {
	return withJournalLock(j, func() ([]JournalEvent, error) {
		return j.eventsLocked(engramID)
	})
}

func (j *Journal) eventsLocked(engramID string) ([]JournalEvent, error) {
	file, err := os.Open(filepath.Join(j.engramDir(engramID), "journal.jsonl"))
	if os.IsNotExist(err) {
		return []JournalEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxJournalBytes {
		return nil, fmt.Errorf("Engram Journal is %d bytes and exceeds the 128 MiB v0 limit; refusing to wake with incomplete history", info.Size())
	}
	var events []JournalEvent
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	lineNumber := 0
	formatGuardSeen := false
	for scanner.Scan() {
		lineNumber++
		line := scanner.Bytes()
		var marker string
		if err := json.Unmarshal(line, &marker); err == nil {
			if marker != journalFormatMarker {
				return nil, fmt.Errorf("decode Journal line %d: unsupported format marker %q", lineNumber, marker)
			}
			formatGuardSeen = true
			continue
		}
		var event JournalEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("decode Journal line %d: %w", lineNumber, err)
		}
		if event.ProtocolVersion != ProtocolVersion {
			return nil, fmt.Errorf("decode Journal line %d: unsupported protocol_version %q (runtime supports %q)", lineNumber, event.ProtocolVersion, ProtocolVersion)
		}
		if !knownJournalEventKind(event.Kind) {
			return nil, fmt.Errorf("decode Journal line %d: unsupported event kind %q", lineNumber, event.Kind)
		}
		if isV03JournalEventKind(event.Kind) && !formatGuardSeen {
			return nil, fmt.Errorf("decode Journal line %d: %s event is missing the %s downgrade guard", lineNumber, event.Kind, journalFormatMarker)
		}
		events = append(events, event)
	}
	return events, scanner.Err()
}

func knownJournalEventKind(kind string) bool {
	switch kind {
	case "opened", "wake_result", "observation", "released", "outcome", "self_fold", "self_fold_reverted":
		return true
	default:
		return false
	}
}

func isV03JournalEventKind(kind string) bool {
	return kind == "outcome" || kind == "self_fold" || kind == "self_fold_reverted"
}

func (j *Journal) WriteLocator(accID, engramID string) error {
	return withJournalLockErr(j, func() error {
		return j.writeLocatorLocked(accID, engramID)
	})
}

func (j *Journal) writeLocatorLocked(accID, engramID string) error {
	return writeJSONExclusive(filepath.Join(j.Root, "accompaniments", accID+".json"), map[string]string{"engram_id": engramID})
}

func (j *Journal) Locate(accID string) (string, error) {
	return withJournalLock(j, func() (string, error) {
		return j.locateLocked(accID)
	})
}

func (j *Journal) locateLocked(accID string) (string, error) {
	if err := validateID(accID); err != nil {
		return "", err
	}
	var value map[string]string
	if err := readJSON(filepath.Join(j.Root, "accompaniments", accID+".json"), &value); err != nil {
		return "", fmt.Errorf("find accompaniment %q: %w", accID, err)
	}
	return value["engram_id"], nil
}

func (j *Journal) WriteIndex(kind, key string, value any) error {
	return withJournalLockErr(j, func() error {
		return j.writeIndexLocked(kind, key, value)
	})
}

func (j *Journal) writeIndexLocked(kind, key string, value any) error {
	dir := filepath.Join(j.Root, kind)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, safeKey(key)+".json")
	temp, err := os.CreateTemp(dir, ".index-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	encoder := json.NewEncoder(temp)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(name, path)
}

func (j *Journal) ReadIndex(kind, key string, value any) (bool, error) {
	return withJournalLock(j, func() (bool, error) {
		return j.readIndexLocked(kind, key, value)
	})
}

func (j *Journal) readIndexLocked(kind, key string, value any) (bool, error) {
	err := readJSON(filepath.Join(j.Root, kind, safeKey(key)+".json"), value)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (j *Journal) RemoveIndex(kind, key string) error {
	return withJournalLockErr(j, func() error {
		return j.removeIndexLocked(kind, key)
	})
}

func (j *Journal) removeIndexLocked(kind, key string) error {
	err := os.Remove(filepath.Join(j.Root, kind, safeKey(key)+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (j *Journal) engramDir(id string) string { return filepath.Join(j.Root, "engrams", id) }

func withJournalLock[T any](j *Journal, operation func() (T, error)) (value T, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	lock, err := acquireProcessFileLock(filepath.Join(j.Root, ".journal.lock"))
	if err != nil {
		return value, fmt.Errorf("acquire Journal lock: %w", err)
	}
	defer func() {
		if releaseErr := lock.release(); err == nil && releaseErr != nil {
			err = fmt.Errorf("release Journal lock: %w", releaseErr)
		}
	}()
	return operation()
}

func withJournalLockErr(j *Journal, operation func() error) error {
	_, err := withJournalLock(j, func() (struct{}, error) {
		return struct{}{}, operation()
	})
	return err
}

func writeJSONExclusive(path string, value any) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
func readJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewDecoder(file).Decode(value)
}
func safeKey(value string) string { return fmt.Sprintf("%x", sha256Bytes([]byte(value))) }
