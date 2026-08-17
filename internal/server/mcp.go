package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Lightmaze/engram/internal/engram"
)

type MCP struct {
	Runtime *engram.Runtime
	Journal *engram.Journal
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (server MCP) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)

	for scanner.Scan() {
		var request rpcRequest
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			_ = encoder.Encode(rpcResponse{
				JSONRPC: "2.0",
				Error:   &rpcError{Code: -32700, Message: "Parse error"},
			})
			continue
		}
		if len(request.ID) == 0 {
			continue
		}
		result, requestError := server.handle(ctx, request)
		response := rpcResponse{JSONRPC: "2.0", ID: request.ID, Result: result, Error: requestError}
		if requestError != nil {
			response.Result = nil
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (server MCP) handle(ctx context.Context, request rpcRequest) (any, *rpcError) {
	switch request.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "engram", "version": engram.Version},
			"instructions":    "Summon one named Engram, keep accompaniment_id while it accompanies the task, then release it.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": tools()}, nil
	case "tools/call":
		var call struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(request.Params, &call); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid tool call"}
		}
		value, err := server.CallTool(ctx, call.Name, call.Arguments)
		if err != nil {
			return toolResult(map[string]string{"error": err.Error()}, true), nil
		}
		return toolResult(value, false), nil
	default:
		return nil, &rpcError{Code: -32601, Message: "Method not found"}
	}
}

func (server MCP) CallTool(ctx context.Context, name string, raw json.RawMessage) (any, error) {
	decode := func(target any) error {
		if len(raw) == 0 {
			raw = []byte("{}")
		}
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		return decoder.Decode(target)
	}

	switch name {
	case "engram_list":
		return server.Journal.List()
	case "engram_create":
		var input engram.CreateRequest
		if err := decode(&input); err != nil {
			return nil, err
		}
		value, err := server.Journal.Create(input)
		if err != nil {
			return nil, err
		}
		return engram.Summary{
			ProtocolVersion: engram.ProtocolVersion,
			ID:              value.ID,
			Name:            value.Name,
			CreatedAt:       value.CreatedAt,
		}, nil
	case "engram_summon":
		var input engram.SummonRequest
		if err := decode(&input); err != nil {
			return nil, err
		}
		return server.Runtime.Summon(ctx, input)
	case "engram_wake":
		var input engram.WakeRequest
		if err := decode(&input); err != nil {
			return nil, err
		}
		return server.Runtime.Wake(ctx, input)
	case "engram_observe":
		var input engram.ObserveRequest
		if err := decode(&input); err != nil {
			return nil, err
		}
		return server.Runtime.Observe(input)
	case "engram_release":
		var input engram.ReleaseRequest
		if err := decode(&input); err != nil {
			return nil, err
		}
		return server.Runtime.Release(input)
	case "engram_outcome":
		var input engram.OutcomeRequest
		if err := decode(&input); err != nil {
			return nil, err
		}
		return server.Runtime.Outcome(ctx, input)
	case "engram_fold_status":
		var input engram.FoldStatusRequest
		if err := decode(&input); err != nil {
			return nil, err
		}
		return server.Runtime.FoldStatus(input)
	case "engram_fold_revert":
		var input engram.FoldRevertRequest
		if err := decode(&input); err != nil {
			return nil, err
		}
		return server.Runtime.RevertFold(input)
	case "engram_guardian_take":
		var input engram.GuardianTakeRequest
		if err := decode(&input); err != nil {
			return nil, err
		}
		value, found, err := server.Runtime.TakePending(input.EngramID, input.Host, input.HostSession)
		return map[string]any{"found": found, "pending": value}, err
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}

func toolResult(value any, failed bool) map[string]any {
	raw, _ := json.Marshal(value)
	return map[string]any{
		"content":           []map[string]string{{"type": "text", "text": string(raw)}},
		"structuredContent": value,
		"isError":           failed,
	}
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	value := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		value["required"] = required
	}
	return value
}

func stringSchema(description string) map[string]string {
	return map[string]string{"type": "string", "description": description}
}

func timestampSchema(description string) map[string]string {
	return map[string]string{"type": "string", "format": "date-time", "description": description}
}

func tools() []map[string]any {
	return []map[string]any{
		{
			"name":        "engram_list",
			"description": "List locally available Engrams.",
			"inputSchema": objectSchema(map[string]any{}),
		},
		{
			"name":        "engram_create",
			"description": "Create an Engram from an explicit raw original Agent context.",
			"inputSchema": objectSchema(map[string]any{
				"id":        stringSchema("stable id"),
				"name":      stringSchema("name"),
				"statement": stringSchema("optional catalog note; not used to recreate Engram identity"),
				"messages": map[string]any{
					"type": "array",
					"items": objectSchema(map[string]any{
						"id":          stringSchema("optional id"),
						"role":        stringSchema("role"),
						"content":     stringSchema("exact raw content"),
						"occurred_at": timestampSchema("optional original message timestamp"),
						"source":      stringSchema("optional source"),
					}, "role", "content"),
				},
			}, "id", "name", "messages"),
		},
		{
			"name":        "engram_summon",
			"description": "Wake one named Engram and open a bounded multi-round accompaniment period. Keep the returned accompaniment_id.",
			"inputSchema": objectSchema(map[string]any{
				"engram_id":       stringSchema("Engram id"),
				"reason":          stringSchema("why now"),
				"scene":           stringSchema("current scene"),
				"host":            stringSchema("host name"),
				"host_session_id": stringSchema("host session"),
				"workspace":       stringSchema("workspace"),
				"idle_seconds":    map[string]any{"type": "integer", "minimum": 1, "maximum": 604800},
				"host_turn_id":    stringSchema("optional host turn id"),
				"request_id":      stringSchema("stable retry id"),
			}, "engram_id", "reason", "scene"),
		},
		{
			"name":        "engram_wake",
			"description": "Continue the same accompanying Engram in a changed scene.",
			"inputSchema": objectSchema(map[string]any{
				"accompaniment_id": stringSchema("summon handle"),
				"scene":            stringSchema("changed scene"),
				"host_turn_id":     stringSchema("optional host turn id"),
				"request_id":       stringSchema("optional stable retry id"),
			}, "accompaniment_id", "scene"),
		},
		{
			"name":        "engram_observe",
			"description": "Let the accompanying Engram observe an exact host message or tool result and return its durable observation_event_id.",
			"inputSchema": objectSchema(map[string]any{
				"accompaniment_id": stringSchema("summon handle"),
				"role":             stringSchema("role"),
				"content":          stringSchema("exact content"),
				"host_turn_id":     stringSchema("optional host turn id"),
				"request_id":       stringSchema("optional stable retry id"),
			}, "accompaniment_id", "role", "content"),
		},
		{
			"name":        "engram_release",
			"description": "End an accompaniment period and let the Engram sleep without deleting its Journal.",
			"inputSchema": objectSchema(map[string]any{
				"accompaniment_id": stringSchema("summon handle"),
				"reason":           stringSchema("ending reason"),
				"request_id":       stringSchema("optional stable retry id"),
			}, "accompaniment_id"),
		},
		{
			"name":        "engram_outcome",
			"description": "Record cited external evidence for one wake and let the Engram immediately produce and apply a change or no_change self-fold. No approval queue is created.",
			"inputSchema": objectSchema(map[string]any{
				"accompaniment_id": stringSchema("accompaniment containing the cited wake"),
				"wake_event_id":    stringSchema("exact wake_result event affected by the outcome"),
				"source_kind":      stringSchema("user_message, tool_result, or external_observation"),
				"source_event_id":  stringSchema("required Journal observation event for user_message or tool_result"),
				"source_ref":       stringSchema("required external source location for external_observation"),
				"content":          stringSchema("exact observed content for external_observation"),
				"request_id":       stringSchema("stable retry id; required"),
			}, "accompaniment_id", "wake_event_id", "source_kind", "request_id"),
		},
		{
			"name":        "engram_fold_status",
			"description": "Show the Engram's current self-authored posture and append-only fold/revert history.",
			"inputSchema": objectSchema(map[string]any{
				"engram_id": stringSchema("Engram id"),
			}, "engram_id"),
		},
		{
			"name":        "engram_fold_revert",
			"description": "Append a correction that deactivates the current self-fold and restores its parent posture without deleting history.",
			"inputSchema": objectSchema(map[string]any{
				"engram_id":     stringSchema("Engram id"),
				"fold_event_id": stringSchema("current active self-fold event"),
				"reason":        stringSchema("why the host is returning to the parent posture"),
				"request_id":    stringSchema("stable retry id"),
			}, "engram_id", "fold_event_id", "reason", "request_id"),
		},
		{
			"name":        "engram_guardian_take",
			"description": "Take pending automatic guardian steering when a host Hook cannot inject context directly.",
			"inputSchema": objectSchema(map[string]any{
				"engram_id":       stringSchema("Engram id"),
				"host":            stringSchema("host"),
				"host_session_id": stringSchema("host session"),
			}, "engram_id", "host"),
		},
	}
}
