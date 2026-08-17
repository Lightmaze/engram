package engram

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

func (runtime *Runtime) Outcome(ctx context.Context, request OutcomeRequest) (OutcomeResult, error) {
	return withJournalLock(runtime.Journal, func() (OutcomeResult, error) {
		return runtime.outcomeLocked(ctx, request)
	})
}

func (runtime *Runtime) outcomeLocked(ctx context.Context, request OutcomeRequest) (OutcomeResult, error) {
	if err := validateID(request.AccompanimentID); err != nil {
		return OutcomeResult{}, fmt.Errorf("invalid accompaniment_id: %w", err)
	}
	if err := validateID(request.WakeEventID); err != nil {
		return OutcomeResult{}, fmt.Errorf("invalid wake_event_id: %w", err)
	}
	if err := required("outcome request_id", request.RequestID, 200); err != nil {
		return OutcomeResult{}, err
	}
	if err := validateID(request.RequestID); err != nil {
		return OutcomeResult{}, fmt.Errorf("invalid outcome request_id: %w", err)
	}
	accompaniment, err := runtime.findLocked(request.AccompanimentID)
	if err != nil {
		return OutcomeResult{}, err
	}
	events, err := runtime.Journal.eventsLocked(accompaniment.EngramID)
	if err != nil {
		return OutcomeResult{}, err
	}

	if existing := outcomeByRequestID(events, request.RequestID); existing != nil {
		if err := existingOutcomeMatches(*existing, request); err != nil {
			return OutcomeResult{}, err
		}
		if fold := selfFoldForOutcome(events, existing.ID); fold != nil {
			return outcomeResult(*existing, *fold, events), nil
		}
		return runtime.foldOutcomeLocked(ctx, accompaniment.EngramID, events, *existing)
	}

	wakeIndex, wake := eventIndexByID(events, request.WakeEventID)
	if wake == nil || wake.Kind != "wake_result" || wake.AccompanimentID != accompaniment.ID {
		return OutcomeResult{}, errors.New("outcome wake_event_id must identify a wake_result from the same accompaniment")
	}
	content, sourceEventID, sourceRef, err := outcomeSource(events, wakeIndex, accompaniment.ID, request)
	if err != nil {
		return OutcomeResult{}, err
	}
	outcomeEvent := JournalEvent{
		ProtocolVersion: ProtocolVersion,
		ID:              stableID("evt", accompaniment.EngramID, request.RequestID, "outcome"),
		Kind:            "outcome",
		AccompanimentID: accompaniment.ID,
		OccurredAt:      nowUTC(),
		Content:         content,
		RequestID:       request.RequestID,
		Outcome: &OutcomeRecord{
			WakeEventID:   wake.ID,
			SourceKind:    request.SourceKind,
			SourceEventID: sourceEventID,
			SourceRef:     sourceRef,
			SourceDigest:  fmt.Sprintf("sha256:%x", sha256Bytes([]byte(content))),
		},
	}
	if err := runtime.Journal.appendLocked(accompaniment.EngramID, outcomeEvent); err != nil {
		return OutcomeResult{}, err
	}
	events = append(events, outcomeEvent)
	return runtime.foldOutcomeLocked(ctx, accompaniment.EngramID, events, outcomeEvent)
}

func outcomeSource(events []JournalEvent, wakeIndex int, accompanimentID string, request OutcomeRequest) (content, sourceEventID, sourceRef string, err error) {
	switch request.SourceKind {
	case "user_message", "tool_result":
		if request.SourceRef != "" || request.Content != "" {
			return "", "", "", errors.New("Journal-backed outcome cannot override source_ref or content")
		}
		if err := validateID(request.SourceEventID); err != nil {
			return "", "", "", fmt.Errorf("invalid source_event_id: %w", err)
		}
		sourceIndex, source := eventIndexByID(events, request.SourceEventID)
		if source == nil || source.Kind != "observation" || source.AccompanimentID != accompanimentID || sourceIndex <= wakeIndex {
			return "", "", "", errors.New("outcome source_event_id must identify a later observation from the same accompaniment")
		}
		role := strings.ToLower(strings.TrimSpace(source.Role))
		if request.SourceKind == "user_message" && role != "user" {
			return "", "", "", errors.New("user_message outcome must cite an observation with role=user")
		}
		if request.SourceKind == "tool_result" && role != "toolresult" && role != "tool_result" && role != "tool" {
			return "", "", "", errors.New("tool_result outcome must cite an observation with a tool-result role")
		}
		return source.Content, source.ID, "journal-event:" + source.ID, nil
	case "external_observation":
		if request.SourceEventID != "" {
			return "", "", "", errors.New("external_observation outcome cannot claim a Journal source_event_id")
		}
		if err := required("external outcome source_ref", request.SourceRef, 4096); err != nil {
			return "", "", "", err
		}
		if err := required("external outcome content", request.Content, 4*1024*1024); err != nil {
			return "", "", "", err
		}
		return request.Content, "", request.SourceRef, nil
	default:
		return "", "", "", errors.New("outcome source_kind must be user_message, tool_result, or external_observation")
	}
}

func (runtime *Runtime) foldOutcomeLocked(ctx context.Context, engramID string, events []JournalEvent, outcome JournalEvent) (OutcomeResult, error) {
	if outcome.Kind != "outcome" || outcome.Outcome == nil {
		return OutcomeResult{ProtocolVersion: ProtocolVersion, OutcomeEvent: outcome}, errors.New("cannot self-fold a malformed outcome event")
	}
	value, err := runtime.Journal.loadLocked(engramID)
	if err != nil {
		return OutcomeResult{ProtocolVersion: ProtocolVersion, OutcomeEvent: outcome}, err
	}
	patch, err := runtime.Provider.Fold(ctx, value, events, outcome)
	if err != nil {
		return OutcomeResult{ProtocolVersion: ProtocolVersion, OutcomeEvent: outcome}, fmt.Errorf("outcome %s was persisted but Engram self-fold failed: %w", outcome.ID, err)
	}
	if err := validateSelfFoldPatch(patch); err != nil {
		return OutcomeResult{ProtocolVersion: ProtocolVersion, OutcomeEvent: outcome}, fmt.Errorf("outcome %s was persisted but Engram self-fold was invalid: %w", outcome.ID, err)
	}
	parentID := ""
	if active := currentActiveFold(events); active != nil {
		parentID = active.ID
	}
	basis := []string{outcome.Outcome.WakeEventID, outcome.ID}
	if outcome.Outcome.SourceEventID != "" {
		basis = append(basis, outcome.Outcome.SourceEventID)
	}
	foldEvent := JournalEvent{
		ProtocolVersion: ProtocolVersion,
		ID:              stableID("evt", engramID, outcome.ID, "self-fold"),
		Kind:            "self_fold",
		AccompanimentID: outcome.AccompanimentID,
		OccurredAt:      nowUTC(),
		RequestID:       outcome.RequestID,
		SelfFold: &SelfFoldRecord{
			OutcomeEventID:    outcome.ID,
			ParentFoldEventID: parentID,
			Actor:             "engram:" + engramID,
			Authority:         "posture/hypothesis",
			UserRatified:      false,
			BasisEventIDs:     basis,
			SelfFoldPatch:     patch,
		},
	}
	if err := runtime.Journal.appendLocked(engramID, foldEvent); err != nil {
		return OutcomeResult{ProtocolVersion: ProtocolVersion, OutcomeEvent: outcome}, err
	}
	events = append(events, foldEvent)
	return outcomeResult(outcome, foldEvent, events), nil
}

func outcomeResult(outcome, fold JournalEvent, events []JournalEvent) OutcomeResult {
	activeID := ""
	if active := currentActiveFold(events); active != nil {
		activeID = active.ID
	}
	return OutcomeResult{
		ProtocolVersion: ProtocolVersion,
		OutcomeEvent:    outcome,
		SelfFoldEvent:   fold,
		ActiveFoldID:    activeID,
	}
}

func (runtime *Runtime) FoldStatus(request FoldStatusRequest) (FoldStatus, error) {
	return withJournalLock(runtime.Journal, func() (FoldStatus, error) {
		if _, err := runtime.Journal.loadLocked(request.EngramID); err != nil {
			return FoldStatus{}, err
		}
		events, err := runtime.Journal.eventsLocked(request.EngramID)
		if err != nil {
			return FoldStatus{}, err
		}
		return foldStatusFromEvents(request.EngramID, events), nil
	})
}

func (runtime *Runtime) RevertFold(request FoldRevertRequest) (FoldStatus, error) {
	return withJournalLock(runtime.Journal, func() (FoldStatus, error) {
		return runtime.revertFoldLocked(request)
	})
}

func (runtime *Runtime) revertFoldLocked(request FoldRevertRequest) (FoldStatus, error) {
	if err := validateID(request.EngramID); err != nil {
		return FoldStatus{}, fmt.Errorf("invalid Engram id: %w", err)
	}
	if err := validateID(request.FoldEventID); err != nil {
		return FoldStatus{}, fmt.Errorf("invalid fold_event_id: %w", err)
	}
	if err := required("fold revert reason", request.Reason, 4096); err != nil {
		return FoldStatus{}, err
	}
	if err := validateID(request.RequestID); err != nil {
		return FoldStatus{}, fmt.Errorf("invalid fold revert request_id: %w", err)
	}
	if _, err := runtime.Journal.loadLocked(request.EngramID); err != nil {
		return FoldStatus{}, err
	}
	events, err := runtime.Journal.eventsLocked(request.EngramID)
	if err != nil {
		return FoldStatus{}, err
	}
	for _, event := range events {
		if event.Kind == "self_fold_reverted" && event.RequestID == request.RequestID {
			if event.SelfFoldReverted == nil || event.SelfFoldReverted.FoldEventID != request.FoldEventID || event.SelfFoldReverted.Reason != request.Reason {
				return FoldStatus{}, errors.New("fold revert request_id was already used with different fold or reason")
			}
			return foldStatusFromEvents(request.EngramID, events), nil
		}
	}
	active := currentActiveFold(events)
	if active == nil || active.ID != request.FoldEventID || active.SelfFold == nil {
		return FoldStatus{}, errors.New("fold_event_id must identify the current active self-fold")
	}
	revertEvent := JournalEvent{
		ProtocolVersion: ProtocolVersion,
		ID:              stableID("evt", request.EngramID, request.RequestID, "self-fold-revert"),
		Kind:            "self_fold_reverted",
		OccurredAt:      nowUTC(),
		Reason:          request.Reason,
		RequestID:       request.RequestID,
		SelfFoldReverted: &SelfFoldRevertedRecord{
			FoldEventID:         active.ID,
			RestoredFoldEventID: active.SelfFold.ParentFoldEventID,
			Actor:               "host",
			Reason:              request.Reason,
		},
	}
	if err := runtime.Journal.appendLocked(request.EngramID, revertEvent); err != nil {
		return FoldStatus{}, err
	}
	events = append(events, revertEvent)
	return foldStatusFromEvents(request.EngramID, events), nil
}

func foldStatusFromEvents(engramID string, events []JournalEvent) FoldStatus {
	status := FoldStatus{ProtocolVersion: ProtocolVersion, EngramID: engramID}
	for _, event := range events {
		if event.Kind == "self_fold" || event.Kind == "self_fold_reverted" {
			status.History = append(status.History, event)
		}
	}
	if active := currentActiveFold(events); active != nil {
		status.ActiveFoldID = active.ID
		status.ActiveFold = active
	}
	return status
}

func outcomeByRequestID(events []JournalEvent, requestID string) *JournalEvent {
	for i := range events {
		if events[i].Kind == "outcome" && events[i].RequestID == requestID {
			return &events[i]
		}
	}
	return nil
}

func selfFoldForOutcome(events []JournalEvent, outcomeID string) *JournalEvent {
	for i := range events {
		if events[i].Kind == "self_fold" && events[i].SelfFold != nil && events[i].SelfFold.OutcomeEventID == outcomeID {
			return &events[i]
		}
	}
	return nil
}

func eventIndexByID(events []JournalEvent, id string) (int, *JournalEvent) {
	for i := range events {
		if events[i].ID == id {
			return i, &events[i]
		}
	}
	return -1, nil
}

func existingOutcomeMatches(event JournalEvent, request OutcomeRequest) error {
	if event.Outcome == nil || event.AccompanimentID != request.AccompanimentID || event.Outcome.WakeEventID != request.WakeEventID || event.Outcome.SourceKind != request.SourceKind {
		return errors.New("outcome request_id was already used with different causal fields")
	}
	if request.SourceKind == "external_observation" {
		if event.Outcome.SourceRef != request.SourceRef || event.Content != request.Content {
			return errors.New("outcome request_id was already used with different external evidence")
		}
	} else if event.Outcome.SourceEventID != request.SourceEventID {
		return errors.New("outcome request_id was already used with a different source_event_id")
	}
	return nil
}
