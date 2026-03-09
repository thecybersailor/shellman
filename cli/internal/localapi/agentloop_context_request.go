package localapi

import (
	"strings"

	"github.com/flaboy/agentloop"
)

func buildAgentLoopContextRequest(message, historyBlock, previousResponseID string, store *bool) agentloop.ContextBuildRequest {
	req := agentloop.ContextBuildRequest{
		Inbound: agentloop.InboundMessage{
			Role:    "user",
			Content: strings.TrimSpace(message),
		},
		ConversationHistory: strings.TrimSpace(historyBlock),
		HistoryMode:         agentloop.HistoryModeHybridAuto,
		PreviousResponseID:  strings.TrimSpace(previousResponseID),
	}
	if store != nil {
		value := *store
		req.Store = &value
	}
	return req
}
