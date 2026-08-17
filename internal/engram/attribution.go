package engram

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

const attributedSpeechHeader = "[Engram: attributed Engram speech]"
const truncatedSteeringMarker = "\n[Engram steering truncated by host context limit]"

type textAttribution struct {
	EngramID           string `json:"engram_id"`
	Name               string `json:"name"`
	Statement          string `json:"statement,omitempty"`
	StatementTruncated bool   `json:"statement_truncated,omitempty"`
	AccompanimentID    string `json:"accompaniment_id"`
}

func attributionFor(value Engram, accompaniment Accompaniment) EngramAttribution {
	return EngramAttribution{
		EngramID:        value.ID,
		Name:            value.Name,
		Statement:       value.Statement,
		AccompanimentID: accompaniment.ID,
	}
}

// AttributedSteeringText renders steering for hosts whose Hook protocol can
// inject only text. JSON keeps metadata on one unambiguous line even when a
// user-supplied name or statement contains newlines.
func AttributedSteeringText(result WakeResult) string {
	if strings.TrimSpace(result.Steering) == "" {
		return ""
	}
	metadata, _ := json.Marshal(result.Attribution)
	return attributedSpeechHeader + "\nAttribution: " + string(metadata) + "\nSteering:\n" + result.Steering
}

// AttributedSteeringTextWithin renders a bounded text-only delivery without
// allowing the host to silently cut through attribution metadata. The full
// attribution remains available in structured WakeResult output.
func AttributedSteeringTextWithin(result WakeResult, maxBytes int) string {
	if strings.TrimSpace(result.Steering) == "" || maxBytes <= 0 {
		return ""
	}
	statement, statementTruncated := truncateUTF8(result.Attribution.Statement, 256)
	compact := textAttribution{
		EngramID:           result.Attribution.EngramID,
		Name:               result.Attribution.Name,
		Statement:          statement,
		StatementTruncated: statementTruncated,
		AccompanimentID:    result.Attribution.AccompanimentID,
	}
	metadata, _ := json.Marshal(compact)
	prefix := attributedSpeechHeader + "\nAttribution: " + string(metadata) + "\nSteering:\n"
	if len(prefix)+len(result.Steering) <= maxBytes {
		return prefix + result.Steering
	}
	available := maxBytes - len(prefix) - len(truncatedSteeringMarker)
	if available < 0 {
		compact.Statement = ""
		compact.StatementTruncated = result.Attribution.Statement != ""
		metadata, _ = json.Marshal(compact)
		prefix = attributedSpeechHeader + "\nAttribution: " + string(metadata) + "\nSteering:\n"
		available = maxBytes - len(prefix) - len(truncatedSteeringMarker)
	}
	if available < 0 {
		return ""
	}
	steering, _ := truncateUTF8(result.Steering, available)
	return prefix + steering + truncatedSteeringMarker
}

func truncateUTF8(value string, maxBytes int) (string, bool) {
	if len(value) <= maxBytes {
		return value, false
	}
	if maxBytes < 1 {
		return "", value != ""
	}
	for maxBytes > 0 && !utf8.ValidString(value[:maxBytes]) {
		maxBytes--
	}
	return value[:maxBytes], true
}

func validateAttribution(value EngramAttribution, engramID, accompanimentID string) error {
	if value.EngramID == "" || value.Name == "" || value.AccompanimentID == "" {
		return errors.New("Engram attribution is incomplete")
	}
	if value.EngramID != engramID || value.AccompanimentID != accompanimentID {
		return errors.New("Engram attribution does not match pending steering")
	}
	return nil
}
