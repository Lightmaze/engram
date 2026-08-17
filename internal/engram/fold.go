package engram

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

func continuityMessages(value Engram, events []JournalEvent) []providerMessage {
	messages := make([]providerMessage, 0, len(value.Messages)+len(events)*2+4)
	for _, message := range value.Messages {
		messages = append(messages, providerMessage{Role: message.Role, Content: message.Content})
	}

	hasBoundary := false
	for _, event := range events {
		switch event.Kind {
		case "opened":
			messages = append(messages, providerMessage{Role: "user", Content: wakeBoundary(event.Reason)})
			hasBoundary = true
		case "wake_result":
			if event.Scene != "" {
				messages = append(messages, activeThreadSlice(event.Scene))
			}
			if event.Steering != "" {
				messages = append(messages, providerMessage{Role: "assistant", Content: event.Steering})
			}
		case "observation":
			messages = append(messages, observedThreadMessage(event.Role, event.Content))
		case "released":
			messages = append(messages, releasedAccompanimentMessage(event))
		case "outcome":
			messages = append(messages, outcomeEvidenceMessage(event))
		case "self_fold":
			messages = append(messages, historicalSelfFoldMessages(event)...)
		case "self_fold_reverted":
			messages = append(messages, selfFoldRevertedMessage(event))
		}
	}
	if !hasBoundary {
		messages = append(messages, providerMessage{Role: "user", Content: wakeBoundary("")})
	}
	if active := currentActiveFold(events); active != nil {
		messages = append(messages, activeFoldMessages(*active)...)
	}
	return messages
}

func releasedAccompanimentMessage(event JournalEvent) providerMessage {
	when := "unknown"
	if !event.OccurredAt.IsZero() {
		when = event.OccurredAt.UTC().Format(time.RFC3339Nano)
	}
	return providerMessage{
		Role: "user",
		Content: fmt.Sprintf(
			"[Accompaniment release boundary; accompaniment_id=%s; occurred_at=%s]\nThis records that the prior accompaniment ended. It is not an outcome or a judgment of success.\nReason: %s",
			event.AccompanimentID,
			when,
			event.Reason,
		),
	}
}

func outcomeEvidenceMessage(event JournalEvent) providerMessage {
	if event.Outcome == nil {
		return providerMessage{Role: "user", Content: "[Malformed outcome event omitted from interpretation]"}
	}
	return providerMessage{
		Role: "user",
		Content: fmt.Sprintf(
			"[Outcome evidence; event_id=%s; wake_event_id=%s; source_kind=%s; source_ref=%s; source_digest=%s]\nThis is cited external evidence. Its source kind limits what it proves; it is not a system truth or a user commitment.\n%s",
			event.ID,
			event.Outcome.WakeEventID,
			event.Outcome.SourceKind,
			event.Outcome.SourceRef,
			event.Outcome.SourceDigest,
			event.Content,
		),
	}
}

func activeFoldMessages(event JournalEvent) []providerMessage {
	if event.SelfFold == nil {
		return nil
	}
	raw, err := json.Marshal(event.SelfFold)
	if err != nil {
		return nil
	}
	return []providerMessage{
		{
			Role:    "user",
			Content: fmt.Sprintf("[Engram active self-fold boundary; event_id=%s]\nThe following posture is this Engram's current self-authored hypothesis, derived from cited outcomes. It is not a user statement, user commitment, or independently proven fact.", event.ID),
		},
		{Role: "assistant", Content: string(raw)},
	}
}

func historicalSelfFoldMessages(event JournalEvent) []providerMessage {
	if event.SelfFold == nil {
		return []providerMessage{{Role: "user", Content: "[Malformed historical self-fold omitted from interpretation]"}}
	}
	raw, err := json.Marshal(event.SelfFold)
	if err != nil {
		return []providerMessage{{Role: "user", Content: "[Unserializable historical self-fold omitted from interpretation]"}}
	}
	return []providerMessage{
		{
			Role: "user",
			Content: fmt.Sprintf(
				"[Engram historical self-fold; event_id=%s]\nThis records a posture authored by this Engram at that point in its experience. It is not automatically current: later folds, corrections, and the final active-fold boundary determine the posture now in force.",
				event.ID,
			),
		},
		{Role: "assistant", Content: string(raw)},
	}
}

func selfFoldRevertedMessage(event JournalEvent) providerMessage {
	if event.SelfFoldReverted == nil {
		return providerMessage{Role: "user", Content: "[Malformed self-fold correction omitted from interpretation]"}
	}
	return providerMessage{
		Role: "user",
		Content: fmt.Sprintf(
			"[Self-fold correction; event_id=%s; fold_event_id=%s; restored_fold_event_id=%s]\nThe host deactivated that self-fold and restored its parent posture. This is a durable correction, not deletion of the experience and not proof that every cited outcome was false.\nReason: %s",
			event.ID,
			event.SelfFoldReverted.FoldEventID,
			event.SelfFoldReverted.RestoredFoldEventID,
			event.SelfFoldReverted.Reason,
		),
	}
}

func selfFoldMessages(value Engram, events []JournalEvent, outcome JournalEvent) []providerMessage {
	messages := continuityMessages(value, events)
	messages = append(messages, providerMessage{
		Role: "user",
		Content: fmt.Sprintf(`[Engram self-fold request]
Outcome event %s cites a concrete earlier wake and an allowed external source. Decide whether that outcome should change how you carry your own history into later scenes.

This is your self-update, not a user review. Do not rewrite or summarize away the original messages. Do not turn your own prior steering, the Main Agent's confidence, release, or user silence into fact. Keep every conclusion scoped to what the cited source can establish.

Return exactly one JSON object with these keys:
{
  "decision": "change" | "no_change",
  "update_intent": "why this outcome does or does not warrant a durable self-update",
  "posture": "for change: the complete current posture you will carry forward; for no_change: omit or leave empty",
  "what_to_keep": ["durable commitments retained"],
  "what_to_fold": ["parts of this experience integrated now"],
  "what_to_expand_next_time": ["questions or evidence to examine later"],
  "activation_boundary_delta": ["scoped situations that should make you more likely to speak"],
  "silence_boundary_delta": ["scoped situations that should make you more likely to remain silent"],
  "relation_to_previous_experience": "how this relates to the cited wake and any current posture",
  "expected_delta": "for change: how a later judgment should differ; for no_change: omit or leave empty"
}

Use no_change when the outcome adds no durable lesson. Do not include Markdown fences or any text outside the JSON object.`, outcome.ID),
	})
	return messages
}

func selfFoldFromText(value string) (SelfFoldPatch, error) {
	value = strings.TrimSpace(value)
	start := strings.IndexByte(value, '{')
	end := strings.LastIndexByte(value, '}')
	if start < 0 || end < start {
		return SelfFoldPatch{}, errors.New("Engram self-fold provider returned no JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(value[start : end+1])))
	decoder.DisallowUnknownFields()
	var patch SelfFoldPatch
	if err := decoder.Decode(&patch); err != nil {
		return SelfFoldPatch{}, fmt.Errorf("decode Engram self-fold: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return SelfFoldPatch{}, errors.New("Engram self-fold must contain exactly one JSON object")
	}
	if err := validateSelfFoldPatch(patch); err != nil {
		return SelfFoldPatch{}, err
	}
	return patch, nil
}

func validateSelfFoldPatch(patch SelfFoldPatch) error {
	if patch.Decision != "change" && patch.Decision != "no_change" {
		return errors.New("Engram self-fold decision must be change or no_change")
	}
	if err := required("Engram self-fold update_intent", patch.UpdateIntent, 4096); err != nil {
		return err
	}
	if err := required("Engram self-fold relation_to_previous_experience", patch.RelationToPreviousExperience, 4096); err != nil {
		return err
	}
	if patch.Decision == "change" {
		if err := required("Engram self-fold posture", patch.Posture, 16384); err != nil {
			return err
		}
		if err := required("Engram self-fold expected_delta", patch.ExpectedDelta, 8192); err != nil {
			return err
		}
	} else if strings.TrimSpace(patch.Posture) != "" || strings.TrimSpace(patch.ExpectedDelta) != "" {
		return errors.New("no_change self-fold cannot replace the current posture or expected delta")
	}
	for name, values := range map[string][]string{
		"what_to_keep":              patch.WhatToKeep,
		"what_to_fold":              patch.WhatToFold,
		"what_to_expand_next_time":  patch.WhatToExpandNextTime,
		"activation_boundary_delta": patch.ActivationBoundaryDelta,
		"silence_boundary_delta":    patch.SilenceBoundaryDelta,
	} {
		if len(values) > 16 {
			return fmt.Errorf("Engram self-fold %s exceeds 16 items", name)
		}
		for _, item := range values {
			if err := required("Engram self-fold "+name+" item", item, 2048); err != nil {
				return err
			}
		}
	}
	return nil
}

func currentActiveFold(events []JournalEvent) *JournalEvent {
	folds := make(map[string]JournalEvent)
	activeID := ""
	for _, event := range events {
		switch event.Kind {
		case "self_fold":
			if event.SelfFold == nil {
				continue
			}
			folds[event.ID] = event
			if event.SelfFold.Decision == "change" {
				activeID = event.ID
			}
		case "self_fold_reverted":
			if event.SelfFoldReverted != nil {
				activeID = event.SelfFoldReverted.RestoredFoldEventID
			}
		}
	}
	if activeID == "" {
		return nil
	}
	event, found := folds[activeID]
	if !found {
		return nil
	}
	return &event
}
