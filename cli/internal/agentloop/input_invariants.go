package agentloop

import (
	"fmt"
	"strings"
)

func ValidateResponseInputInvariants(input any) error {
	items, ok := normalizeResponseInputItems(input)
	if !ok {
		return nil
	}

	systemCount := 0
	systemIndex := -1
	seenCallIDs := map[string]struct{}{}

	for i, item := range items {
		itemType := fieldString(item["type"])
		role := fieldString(item["role"])

		if itemType == "message" && role == "system" {
			systemCount++
			systemIndex = i
		}

		if itemType == "function_call" {
			callID := fieldString(item["call_id"])
			if callID != "" {
				seenCallIDs[callID] = struct{}{}
			}
		}

		if itemType == "function_call_output" {
			callID := fieldString(item["call_id"])
			if callID == "" {
				return fmt.Errorf("function_call_output missing call_id at index=%d", i)
			}
			if _, exists := seenCallIDs[callID]; !exists {
				return fmt.Errorf("function_call_output without prior function_call call_id=%q index=%d", callID, i)
			}
		}
	}

	if systemCount > 1 {
		return fmt.Errorf("responses input must contain at most one system message, got=%d", systemCount)
	}
	if systemCount == 1 && systemIndex != 0 {
		return fmt.Errorf("responses input system message must be first, got index=%d", systemIndex)
	}
	return nil
}

func fieldString(v any) string {
	value := strings.TrimSpace(fmt.Sprint(v))
	if value == "<nil>" {
		return ""
	}
	return value
}

func normalizeResponseInputItems(input any) ([]map[string]any, bool) {
	switch v := input.(type) {
	case []map[string]any:
		return v, true
	case []any:
		items := make([]map[string]any, 0, len(v))
		for _, raw := range v {
			item, ok := raw.(map[string]any)
			if !ok {
				return nil, false
			}
			items = append(items, item)
		}
		return items, true
	default:
		return nil, false
	}
}
