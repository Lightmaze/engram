package engram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Runtime struct {
	Journal  *Journal
	Provider Provider
}

func NewRuntime(journal *Journal, provider Provider) (*Runtime, error) {
	if journal == nil || provider == nil {
		return nil, errors.New("Journal and provider are required")
	}
	return &Runtime{Journal: journal, Provider: provider}, nil
}

func (runtime *Runtime) Summon(ctx context.Context, request SummonRequest) (SummonResult, error) {
	return withJournalLock(runtime.Journal, func() (SummonResult, error) {
		return runtime.summonLocked(ctx, request)
	})
}

func (runtime *Runtime) summonLocked(ctx context.Context, request SummonRequest) (SummonResult, error) {
	if _, err := runtime.Journal.loadLocked(request.EngramID); err != nil {
		return SummonResult{}, err
	}
	if err := required("summon reason", request.Reason, 4096); err != nil {
		return SummonResult{}, err
	}
	if err := required("scene", request.Scene, 4*1024*1024); err != nil {
		return SummonResult{}, err
	}
	applySummonDefaults(&request)
	if request.IdleSeconds < 1 || request.IdleSeconds > 604800 {
		return SummonResult{}, errors.New("idle_seconds must be 1-604800")
	}

	id := randomID("acc")
	if request.RequestID != "" {
		id = stableID("acc", request.EngramID, request.Host, request.HostSession, request.RequestID)
	}
	now := nowUTC()
	accompaniment := Accompaniment{
		ProtocolVersion: ProtocolVersion,
		ID:              id,
		EngramID:        request.EngramID,
		Mode:            request.Mode,
		Host: HostRef{
			Kind:      request.Host,
			SessionID: request.HostSession,
			Workspace: request.Workspace,
		},
		Reason:       request.Reason,
		OpenedAt:     now,
		LastActiveAt: now,
		IdleSeconds:  request.IdleSeconds,
		Status:       "active",
	}

	if err := runtime.Journal.writeLocatorLocked(id, request.EngramID); err != nil {
		if request.RequestID == "" {
			return SummonResult{}, err
		}
		return runtime.replaySummonLocked(ctx, request, id, err)
	}
	if err := runtime.Journal.appendLocked(request.EngramID, JournalEvent{
		Kind:            "opened",
		AccompanimentID: id,
		Reason:          request.Reason,
		RequestID:       request.RequestID,
		Accompaniment:   &accompaniment,
	}); err != nil {
		return SummonResult{}, err
	}
	wake, err := runtime.wakeLocked(ctx, WakeRequest{
		AccompanimentID: id,
		Scene:           request.Scene,
		HostTurnID:      request.HostTurnID,
		RequestID:       request.RequestID + "-wake",
	})
	return SummonResult{Accompaniment: wake.Accompaniment, Wake: wake}, err
}

func applySummonDefaults(request *SummonRequest) {
	if request.Host == "" {
		request.Host = "mcp"
	}
	if request.HostSession == "" {
		request.HostSession = "manual"
	}
	if request.IdleSeconds == 0 {
		request.IdleSeconds = 1800
	}
	if request.Mode == "" {
		request.Mode = "summon"
	}
}

func (runtime *Runtime) replaySummonLocked(ctx context.Context, request SummonRequest, id string, locatorErr error) (SummonResult, error) {
	if _, err := runtime.Journal.locateLocked(id); err != nil {
		return SummonResult{}, locatorErr
	}
	existing, err := runtime.findLocked(id)
	if err != nil {
		return SummonResult{}, err
	}
	wake, err := runtime.wakeLocked(ctx, WakeRequest{
		AccompanimentID: id,
		Scene:           request.Scene,
		HostTurnID:      request.HostTurnID,
		RequestID:       request.RequestID + "-wake",
	})
	return SummonResult{Accompaniment: existing, Wake: wake}, err
}

func (runtime *Runtime) Wake(ctx context.Context, request WakeRequest) (WakeResult, error) {
	return withJournalLock(runtime.Journal, func() (WakeResult, error) {
		return runtime.wakeLocked(ctx, request)
	})
}

func (runtime *Runtime) wakeLocked(ctx context.Context, request WakeRequest) (WakeResult, error) {
	if err := required("scene", request.Scene, 4*1024*1024); err != nil {
		return WakeResult{}, err
	}
	accompaniment, err := runtime.findLocked(request.AccompanimentID)
	if err != nil {
		return WakeResult{}, err
	}
	value, err := runtime.Journal.loadLocked(accompaniment.EngramID)
	if err != nil {
		return WakeResult{}, err
	}
	events, err := runtime.Journal.eventsLocked(accompaniment.EngramID)
	if err != nil {
		return WakeResult{}, err
	}
	if request.RequestID != "" {
		if index, existing := eventByRequestID(events, "wake_result", accompaniment.ID, request.RequestID); existing != nil {
			if existing.Scene != request.Scene || existing.HostTurnID != request.HostTurnID {
				return WakeResult{}, errors.New("wake request_id was already used with different scene or host_turn_id")
			}
			return wakeResultFromEvent(value, accompaniment, events[:index], *existing), nil
		}
	}
	if accompaniment.Status != "active" {
		return WakeResult{}, fmt.Errorf("accompaniment %q is sleeping", accompaniment.ID)
	}
	decision, err := runtime.Provider.Wake(ctx, value, events, request.Scene)
	if err != nil {
		return WakeResult{}, err
	}
	accompaniment.LastActiveAt = nowUTC()
	wakeEvent := JournalEvent{
		ID:              randomID("evt"),
		Kind:            "wake_result",
		AccompanimentID: accompaniment.ID,
		Scene:           request.Scene,
		State:           decision.State,
		Reason:          decision.Reason,
		Steering:        decision.Steering,
		HostTurnID:      request.HostTurnID,
		RequestID:       request.RequestID,
	}
	if request.RequestID != "" {
		wakeEvent.ID = stableID("evt", accompaniment.EngramID, accompaniment.ID, "wake", request.RequestID)
	}
	if err := runtime.Journal.appendLocked(accompaniment.EngramID, wakeEvent); err != nil {
		return WakeResult{}, err
	}
	return wakeResultFromEvent(value, accompaniment, events, wakeEvent), nil
}

func wakeResultFromEvent(value Engram, accompaniment Accompaniment, priorEvents []JournalEvent, event JournalEvent) WakeResult {
	activeFoldID := ""
	if active := currentActiveFold(priorEvents); active != nil {
		activeFoldID = active.ID
	}
	return WakeResult{
		ProtocolVersion: ProtocolVersion,
		Accompaniment:   accompaniment,
		Attribution:     attributionFor(value, accompaniment),
		WakeEventID:     event.ID,
		ActiveFoldID:    activeFoldID,
		State:           event.State,
		Reason:          event.Reason,
		Steering:        event.Steering,
	}
}

func (runtime *Runtime) Observe(request ObserveRequest) (ObserveResult, error) {
	return withJournalLock(runtime.Journal, func() (ObserveResult, error) {
		return runtime.observeLocked(request)
	})
}

func (runtime *Runtime) observeLocked(request ObserveRequest) (ObserveResult, error) {
	if err := required("observation role", request.Role, 80); err != nil {
		return ObserveResult{}, err
	}
	if err := required("observation content", request.Content, 4*1024*1024); err != nil {
		return ObserveResult{}, err
	}
	accompaniment, err := runtime.findLocked(request.AccompanimentID)
	if err != nil {
		return ObserveResult{}, err
	}
	if request.RequestID != "" {
		events, err := runtime.Journal.eventsLocked(accompaniment.EngramID)
		if err != nil {
			return ObserveResult{}, err
		}
		if _, existing := eventByRequestID(events, "observation", accompaniment.ID, request.RequestID); existing != nil {
			if existing.Role != request.Role || existing.Content != request.Content || existing.HostTurnID != request.HostTurnID {
				return ObserveResult{}, errors.New("observation request_id was already used with different role, content, or host_turn_id")
			}
			return ObserveResult{Accompaniment: accompaniment, ObservationEventID: existing.ID}, nil
		}
	}
	if accompaniment.Status != "active" {
		return ObserveResult{}, errors.New("accompaniment is sleeping")
	}
	observationEvent := JournalEvent{
		ID:              randomID("evt"),
		Kind:            "observation",
		AccompanimentID: accompaniment.ID,
		Role:            request.Role,
		Content:         request.Content,
		HostTurnID:      request.HostTurnID,
		RequestID:       request.RequestID,
	}
	if request.RequestID != "" {
		observationEvent.ID = stableID("evt", accompaniment.EngramID, accompaniment.ID, "observation", request.RequestID)
	}
	if err := runtime.Journal.appendLocked(accompaniment.EngramID, observationEvent); err != nil {
		return ObserveResult{}, err
	}
	accompaniment.LastActiveAt = nowUTC()
	return ObserveResult{
		Accompaniment:      accompaniment,
		ObservationEventID: observationEvent.ID,
	}, nil
}

func eventByRequestID(events []JournalEvent, kind, accompanimentID, requestID string) (int, *JournalEvent) {
	for index := range events {
		if events[index].Kind == kind && events[index].AccompanimentID == accompanimentID && events[index].RequestID == requestID {
			return index, &events[index]
		}
	}
	return -1, nil
}

func (runtime *Runtime) Release(request ReleaseRequest) (Accompaniment, error) {
	return withJournalLock(runtime.Journal, func() (Accompaniment, error) {
		return runtime.releaseLocked(request)
	})
}

func (runtime *Runtime) releaseLocked(request ReleaseRequest) (Accompaniment, error) {
	accompaniment, err := runtime.findLocked(request.AccompanimentID)
	if err != nil {
		return Accompaniment{}, err
	}
	if accompaniment.Status == "sleeping" {
		return accompaniment, nil
	}
	if request.Reason == "" {
		request.Reason = "released"
	}
	now := nowUTC()
	accompaniment.Status = "sleeping"
	accompaniment.SleepReason = request.Reason
	accompaniment.SleptAt = now
	accompaniment.LastActiveAt = now
	if err := runtime.Journal.appendLocked(accompaniment.EngramID, JournalEvent{
		Kind:            "released",
		AccompanimentID: accompaniment.ID,
		Reason:          request.Reason,
		RequestID:       request.RequestID,
	}); err != nil {
		return Accompaniment{}, err
	}
	return accompaniment, nil
}

func (runtime *Runtime) Find(id string) (Accompaniment, error) {
	return withJournalLock(runtime.Journal, func() (Accompaniment, error) {
		return runtime.findLocked(id)
	})
}

func (runtime *Runtime) findLocked(id string) (Accompaniment, error) {
	engramID, err := runtime.Journal.locateLocked(id)
	if err != nil {
		return Accompaniment{}, err
	}
	events, err := runtime.Journal.eventsLocked(engramID)
	if err != nil {
		return Accompaniment{}, err
	}
	var accompaniment Accompaniment
	found := false
	for _, event := range events {
		if event.AccompanimentID != id {
			continue
		}
		switch event.Kind {
		case "opened":
			if event.Accompaniment != nil {
				accompaniment = *event.Accompaniment
				found = true
			}
		case "wake_result", "observation":
			accompaniment.LastActiveAt = event.OccurredAt
		case "released":
			accompaniment.Status = "sleeping"
			accompaniment.SleepReason = event.Reason
			accompaniment.SleptAt = event.OccurredAt
			accompaniment.LastActiveAt = event.OccurredAt
		}
	}
	if !found {
		return Accompaniment{}, fmt.Errorf("accompaniment %q not found", id)
	}
	if accompaniment.Status == "active" {
		deadline := accompaniment.LastActiveAt.Add(time.Duration(accompaniment.IdleSeconds) * time.Second)
		if nowUTC().After(deadline) {
			accompaniment.Status = "sleeping"
			accompaniment.SleepReason = "idle timeout"
			accompaniment.SleptAt = deadline
		}
	}
	return accompaniment, nil
}

func (runtime *Runtime) GuardianWake(ctx context.Context, engramID, host, session, workspace, turnID, scene string, idleSeconds int64) (WakeResult, error) {
	return withJournalLock(runtime.Journal, func() (WakeResult, error) {
		return runtime.guardianWakeLocked(ctx, engramID, host, session, workspace, turnID, scene, idleSeconds)
	})
}

func (runtime *Runtime) guardianWakeLocked(ctx context.Context, engramID, host, session, workspace, turnID, scene string, idleSeconds int64) (WakeResult, error) {
	key := guardianKey(host, session, engramID)
	var index struct {
		AccompanimentID string `json:"accompaniment_id"`
	}
	found, err := runtime.Journal.readIndexLocked("guardians", key, &index)
	if err != nil {
		return WakeResult{}, fmt.Errorf("read guardian accompaniment index: %w", err)
	}
	if found {
		accompaniment, err := runtime.findLocked(index.AccompanimentID)
		if err != nil {
			return WakeResult{}, fmt.Errorf("load indexed guardian accompaniment: %w", err)
		}
		if accompaniment.Status == "active" {
			return runtime.wakeLocked(ctx, WakeRequest{AccompanimentID: index.AccompanimentID, Scene: scene, HostTurnID: turnID})
		}
	}
	result, err := runtime.summonLocked(ctx, SummonRequest{
		EngramID:    engramID,
		Reason:      "automatic guardian",
		Scene:       scene,
		Host:        host,
		HostSession: session,
		Workspace:   workspace,
		HostTurnID:  turnID,
		IdleSeconds: idleSeconds,
		Mode:        "guardian",
	})
	if err != nil {
		return WakeResult{}, err
	}
	if err := runtime.Journal.writeIndexLocked("guardians", key, map[string]string{"accompaniment_id": result.Accompaniment.ID}); err != nil {
		_, releaseErr := runtime.releaseLocked(ReleaseRequest{
			AccompanimentID: result.Accompaniment.ID,
			Reason:          "guardian index persistence failed",
		})
		if releaseErr != nil {
			return WakeResult{}, fmt.Errorf("persist guardian accompaniment index: %w; cleanup release also failed: %v", err, releaseErr)
		}
		return WakeResult{}, fmt.Errorf("persist guardian accompaniment index: %w", err)
	}
	return result.Wake, nil
}

func (runtime *Runtime) GuardianObserve(host, session, engramID, role, content string) error {
	return withJournalLockErr(runtime.Journal, func() error {
		return runtime.guardianObserveLocked(host, session, engramID, role, content)
	})
}

func (runtime *Runtime) guardianObserveLocked(host, session, engramID, role, content string) error {
	key := guardianKey(host, session, engramID)
	var index struct {
		AccompanimentID string `json:"accompaniment_id"`
	}
	found, err := runtime.Journal.readIndexLocked("guardians", key, &index)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("guardian accompaniment index not found for host %q session %q Engram %q", host, session, engramID)
	}
	_, err = runtime.observeLocked(ObserveRequest{AccompanimentID: index.AccompanimentID, Role: role, Content: content})
	return err
}

func (runtime *Runtime) GuardianRelease(host, session, engramID string) error {
	return withJournalLockErr(runtime.Journal, func() error {
		return runtime.guardianReleaseLocked(host, session, engramID)
	})
}

func (runtime *Runtime) guardianReleaseLocked(host, session, engramID string) error {
	key := guardianKey(host, session, engramID)
	var index struct {
		AccompanimentID string `json:"accompaniment_id"`
	}
	found, err := runtime.Journal.readIndexLocked("guardians", key, &index)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	accompaniment, err := runtime.findLocked(index.AccompanimentID)
	if err != nil {
		return err
	}
	if accompaniment.Status == "active" {
		if _, err := runtime.releaseLocked(ReleaseRequest{AccompanimentID: index.AccompanimentID, Reason: "host session ended"}); err != nil {
			return err
		}
	}
	return runtime.Journal.removeIndexLocked("guardians", key)
}

func (runtime *Runtime) SavePending(value PendingSteering) error {
	if err := validateAttribution(value.Attribution, value.EngramID, value.AccompanimentID); err != nil {
		return err
	}
	return withJournalLockErr(runtime.Journal, func() error {
		return runtime.Journal.writeIndexLocked("pending", guardianKey(value.Host.Kind, value.Host.SessionID, value.EngramID), value)
	})
}

func (runtime *Runtime) TakePending(engramID, host, session string) (PendingSteering, bool, error) {
	type pendingResult struct {
		value PendingSteering
		found bool
	}
	result, err := withJournalLock(runtime.Journal, func() (pendingResult, error) {
		value, found, err := runtime.takePendingLocked(engramID, host, session)
		return pendingResult{value: value, found: found}, err
	})
	return result.value, result.found, err
}

func (runtime *Runtime) takePendingLocked(engramID, host, session string) (PendingSteering, bool, error) {
	key := guardianKey(host, session, engramID)
	var value PendingSteering
	found, err := runtime.Journal.readIndexLocked("pending", key, &value)
	if err != nil || !found {
		return value, found, err
	}
	if value.Attribution.EngramID == "" && value.Attribution.Name == "" && value.Attribution.AccompanimentID == "" {
		engramValue, err := runtime.Journal.loadLocked(value.EngramID)
		if err != nil {
			return value, true, err
		}
		value.Attribution = attributionFor(engramValue, Accompaniment{ID: value.AccompanimentID})
	}
	if err := validateAttribution(value.Attribution, value.EngramID, value.AccompanimentID); err != nil {
		return value, true, err
	}
	return value, true, runtime.Journal.removeIndexLocked("pending", key)
}

func guardianKey(host, session, engramID string) string {
	return host + "\x00" + session + "\x00" + engramID
}

func stableID(prefix string, parts ...string) string {
	digest := fmt.Sprintf("%x", sha256Bytes([]byte(strings.Join(parts, "\x00"))))
	return prefix + "-" + digest[:32]
}
