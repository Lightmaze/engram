package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Lightmaze/engram/internal/engram"
)

type Hook struct {
	Runtime     *engram.Runtime
	EngramID    string
	IdleSeconds int64
}

const codexAdditionalContextLimit = 5000

type hookEvent struct {
	HookEventName  string  `json:"hook_event_name"`
	Event          string  `json:"event"`
	SessionID      string  `json:"session_id"`
	ConversationID string  `json:"conversation_id"`
	TurnID         string  `json:"turn_id"`
	Prompt         string  `json:"prompt"`
	Text           string  `json:"text"`
	Content        string  `json:"content"`
	Role           string  `json:"role"`
	CWD            string  `json:"cwd"`
	LastAssistant  *string `json:"last_assistant_message"`
}

func (hook Hook) Handle(ctx context.Context, host string, raw []byte) ([]byte, error) {
	if hook.Runtime == nil || strings.TrimSpace(hook.EngramID) == "" {
		return nil, errors.New("guardian runtime and Engram id are required")
	}
	var input hookEvent
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	session := input.SessionID
	if session == "" {
		session = input.ConversationID
	}
	eventName := input.HookEventName
	if eventName == "" {
		eventName = input.Event
	}
	idleSeconds := hook.IdleSeconds
	if idleSeconds == 0 {
		idleSeconds = 1800
	}

	switch eventName {
	case "UserPromptSubmit", "before_prompt", "beforeSubmitPrompt":
		return hook.beforePrompt(ctx, host, session, idleSeconds, input)
	case "Stop", "after_response", "afterAgentResponse":
		return hook.afterResponse(host, session, input)
	case "SessionEnd", "session_end", "sessionEnd":
		if err := hook.Runtime.GuardianRelease(host, session, hook.EngramID); err != nil {
			return nil, err
		}
		return []byte("{}"), nil
	case "sessionStart":
		return json.Marshal(map[string]any{
			"additional_context": "Engram guardian is enabled. At each turn start, call engram_guardian_take for any pending historical steering.",
		})
	default:
		return []byte("{}"), nil
	}
}

func (hook Hook) beforePrompt(ctx context.Context, host, session string, idleSeconds int64, input hookEvent) ([]byte, error) {
	scene := input.Prompt
	if scene == "" {
		scene = input.Content
	}
	if scene == "" {
		scene = input.Text
	}
	if strings.TrimSpace(scene) == "" {
		return nil, errors.New("Hook omitted prompt")
	}
	result, err := hook.Runtime.GuardianWake(ctx, hook.EngramID, host, session, input.CWD, input.TurnID, scene, idleSeconds)
	if err != nil {
		return nil, err
	}
	if host == "cursor" {
		if strings.TrimSpace(result.Steering) == "" {
			if _, _, err := hook.Runtime.TakePending(hook.EngramID, host, session); err != nil {
				return nil, err
			}
			return json.Marshal(map[string]any{"continue": true})
		}
		pending := engram.PendingSteering{
			ProtocolVersion: engram.ProtocolVersion,
			AccompanimentID: result.Accompaniment.ID,
			EngramID:        hook.EngramID,
			Attribution:     result.Attribution,
			Host:            result.Accompaniment.Host,
			State:           result.State,
			Steering:        result.Steering,
			CreatedAt:       time.Now().UTC(),
		}
		if err := hook.Runtime.SavePending(pending); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"continue": true})
	}
	attributedSteering := engram.AttributedSteeringText(result)
	if host == "codex" {
		attributedSteering = engram.AttributedSteeringTextWithin(result, codexAdditionalContextLimit)
	}
	if host == "pi" || host == "generic" {
		return json.Marshal(map[string]any{
			"protocol_version":   "engram-hook/v0",
			"action":             "continue",
			"additional_context": attributedSteering,
			"accompaniment_id":   result.Accompaniment.ID,
			"attribution":        result.Attribution,
			"wake_state":         result.State,
		})
	}
	if result.Steering == "" {
		return []byte("{}"), nil
	}
	return json.Marshal(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "UserPromptSubmit",
			"additionalContext": attributedSteering,
		},
	})
}

func (hook Hook) afterResponse(host, session string, input hookEvent) ([]byte, error) {
	content := input.Content
	if content == "" {
		content = input.Text
	}
	if content == "" && input.LastAssistant != nil {
		content = *input.LastAssistant
	}
	role := strings.TrimSpace(input.Role)
	if role == "" {
		role = "assistant"
	}
	if strings.TrimSpace(content) != "" {
		if err := hook.Runtime.GuardianObserve(host, session, hook.EngramID, role, content); err != nil {
			return nil, err
		}
	}
	return []byte("{}"), nil
}
