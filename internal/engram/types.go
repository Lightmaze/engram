package engram

import "time"

const (
	Version         = "0.3.0"
	ProtocolVersion = "engram/v0"
)

type Message struct {
	ID         string    `json:"id,omitempty"`
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	OccurredAt time.Time `json:"occurred_at,omitempty"`
	Source     string    `json:"source,omitempty"`
}

type CreateRequest struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Statement string    `json:"statement,omitempty"`
	Messages  []Message `json:"messages"`
}

type Engram struct {
	ProtocolVersion string    `json:"protocol_version"`
	ID              string    `json:"engram_id"`
	Name            string    `json:"name"`
	Statement       string    `json:"statement"`
	Messages        []Message `json:"messages"`
	CreatedAt       time.Time `json:"created_at"`
}

type Summary struct {
	ProtocolVersion string    `json:"protocol_version"`
	ID              string    `json:"engram_id"`
	Name            string    `json:"name"`
	CreatedAt       time.Time `json:"created_at"`
}

type HostRef struct {
	Kind      string `json:"kind"`
	SessionID string `json:"session_id"`
	Workspace string `json:"workspace,omitempty"`
}

type Accompaniment struct {
	ProtocolVersion string    `json:"protocol_version"`
	ID              string    `json:"accompaniment_id"`
	EngramID        string    `json:"engram_id"`
	Mode            string    `json:"mode"`
	Host            HostRef   `json:"host"`
	Reason          string    `json:"reason"`
	OpenedAt        time.Time `json:"opened_at"`
	LastActiveAt    time.Time `json:"last_active_at"`
	IdleSeconds     int64     `json:"idle_seconds"`
	Status          string    `json:"status"`
	SleepReason     string    `json:"sleep_reason,omitempty"`
	SleptAt         time.Time `json:"slept_at,omitempty"`
}

// EngramAttribution identifies the historical participant behind a wake
// result. It is runtime-owned metadata, not text authored by the model and not
// a cryptographic signature.
type EngramAttribution struct {
	EngramID        string `json:"engram_id"`
	Name            string `json:"name"`
	Statement       string `json:"statement,omitempty"`
	AccompanimentID string `json:"accompaniment_id"`
}

type SummonRequest struct {
	EngramID    string `json:"engram_id"`
	Reason      string `json:"reason"`
	Scene       string `json:"scene"`
	Host        string `json:"host,omitempty"`
	HostSession string `json:"host_session_id,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
	HostTurnID  string `json:"host_turn_id,omitempty"`
	IdleSeconds int64  `json:"idle_seconds,omitempty"`
	RequestID   string `json:"request_id,omitempty"`
	Mode        string `json:"-"`
}

type WakeRequest struct {
	AccompanimentID string `json:"accompaniment_id"`
	Scene           string `json:"scene"`
	HostTurnID      string `json:"host_turn_id,omitempty"`
	RequestID       string `json:"request_id,omitempty"`
}

type ObserveRequest struct {
	AccompanimentID string `json:"accompaniment_id"`
	Role            string `json:"role"`
	Content         string `json:"content"`
	HostTurnID      string `json:"host_turn_id,omitempty"`
	RequestID       string `json:"request_id,omitempty"`
}

type ReleaseRequest struct {
	AccompanimentID string `json:"accompaniment_id"`
	Reason          string `json:"reason,omitempty"`
	RequestID       string `json:"request_id,omitempty"`
}

type GuardianTakeRequest struct {
	EngramID    string `json:"engram_id"`
	Host        string `json:"host"`
	HostSession string `json:"host_session_id,omitempty"`
}

type WakeResult struct {
	ProtocolVersion string            `json:"protocol_version"`
	Accompaniment   Accompaniment     `json:"accompaniment"`
	Attribution     EngramAttribution `json:"attribution"`
	WakeEventID     string            `json:"wake_event_id"`
	ActiveFoldID    string            `json:"active_fold_event_id,omitempty"`
	State           string            `json:"state"`
	Reason          string            `json:"reason"`
	Steering        string            `json:"steering,omitempty"`
}

type SummonResult struct {
	Accompaniment Accompaniment `json:"accompaniment"`
	Wake          WakeResult    `json:"wake"`
}

type ObserveResult struct {
	// Embed Accompaniment so the public JSON remains backward-compatible with
	// v0.2.0 while adding the durable event id needed by outcome attribution.
	Accompaniment
	ObservationEventID string `json:"observation_event_id"`
}

type OutcomeRequest struct {
	AccompanimentID string `json:"accompaniment_id"`
	WakeEventID     string `json:"wake_event_id"`
	SourceKind      string `json:"source_kind"`
	SourceEventID   string `json:"source_event_id,omitempty"`
	SourceRef       string `json:"source_ref,omitempty"`
	Content         string `json:"content,omitempty"`
	RequestID       string `json:"request_id"`
}

type FoldStatusRequest struct {
	EngramID string `json:"engram_id"`
}

type FoldRevertRequest struct {
	EngramID    string `json:"engram_id"`
	FoldEventID string `json:"fold_event_id"`
	Reason      string `json:"reason"`
	RequestID   string `json:"request_id"`
}

type SelfFoldPatch struct {
	Decision                     string   `json:"decision"`
	UpdateIntent                 string   `json:"update_intent"`
	Posture                      string   `json:"posture,omitempty"`
	WhatToKeep                   []string `json:"what_to_keep,omitempty"`
	WhatToFold                   []string `json:"what_to_fold,omitempty"`
	WhatToExpandNextTime         []string `json:"what_to_expand_next_time,omitempty"`
	ActivationBoundaryDelta      []string `json:"activation_boundary_delta,omitempty"`
	SilenceBoundaryDelta         []string `json:"silence_boundary_delta,omitempty"`
	RelationToPreviousExperience string   `json:"relation_to_previous_experience"`
	ExpectedDelta                string   `json:"expected_delta,omitempty"`
}

type OutcomeRecord struct {
	WakeEventID   string `json:"wake_event_id"`
	SourceKind    string `json:"source_kind"`
	SourceEventID string `json:"source_event_id,omitempty"`
	SourceRef     string `json:"source_ref"`
	SourceDigest  string `json:"source_digest"`
}

type SelfFoldRecord struct {
	OutcomeEventID    string   `json:"outcome_event_id"`
	ParentFoldEventID string   `json:"parent_fold_event_id,omitempty"`
	Actor             string   `json:"actor"`
	Authority         string   `json:"authority"`
	UserRatified      bool     `json:"user_ratified"`
	BasisEventIDs     []string `json:"basis_event_ids"`
	SelfFoldPatch
}

type SelfFoldRevertedRecord struct {
	FoldEventID         string `json:"fold_event_id"`
	RestoredFoldEventID string `json:"restored_fold_event_id,omitempty"`
	Actor               string `json:"actor"`
	Reason              string `json:"reason"`
}

type OutcomeResult struct {
	ProtocolVersion string       `json:"protocol_version"`
	OutcomeEvent    JournalEvent `json:"outcome_event"`
	SelfFoldEvent   JournalEvent `json:"self_fold_event"`
	ActiveFoldID    string       `json:"active_fold_event_id,omitempty"`
}

type FoldStatus struct {
	ProtocolVersion string         `json:"protocol_version"`
	EngramID        string         `json:"engram_id"`
	ActiveFoldID    string         `json:"active_fold_event_id,omitempty"`
	ActiveFold      *JournalEvent  `json:"active_fold,omitempty"`
	History         []JournalEvent `json:"history"`
}

type JournalEvent struct {
	ProtocolVersion  string                  `json:"protocol_version"`
	ID               string                  `json:"event_id"`
	Kind             string                  `json:"kind"`
	AccompanimentID  string                  `json:"accompaniment_id,omitempty"`
	OccurredAt       time.Time               `json:"occurred_at"`
	Role             string                  `json:"role,omitempty"`
	Content          string                  `json:"content,omitempty"`
	Scene            string                  `json:"scene,omitempty"`
	State            string                  `json:"state,omitempty"`
	Reason           string                  `json:"reason,omitempty"`
	Steering         string                  `json:"steering,omitempty"`
	HostTurnID       string                  `json:"host_turn_id,omitempty"`
	RequestID        string                  `json:"request_id,omitempty"`
	Accompaniment    *Accompaniment          `json:"accompaniment,omitempty"`
	Outcome          *OutcomeRecord          `json:"outcome,omitempty"`
	SelfFold         *SelfFoldRecord         `json:"self_fold,omitempty"`
	SelfFoldReverted *SelfFoldRevertedRecord `json:"self_fold_reverted,omitempty"`
}

type Decision struct {
	State    string
	Reason   string
	Steering string
}

type PendingSteering struct {
	ProtocolVersion string            `json:"protocol_version"`
	AccompanimentID string            `json:"accompaniment_id"`
	EngramID        string            `json:"engram_id"`
	Attribution     EngramAttribution `json:"attribution"`
	Host            HostRef           `json:"host"`
	State           string            `json:"state"`
	Steering        string            `json:"steering"`
	CreatedAt       time.Time         `json:"created_at"`
}
