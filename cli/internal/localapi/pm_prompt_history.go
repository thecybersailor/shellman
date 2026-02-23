package localapi

import (
	"strings"

	"shellman/cli/internal/projectstate"
)

func (s *Server) buildPMUserPromptWithMeta(store *projectstate.Store, sessionID, userInput string) (string, TaskPromptHistoryMeta) {
	input := strings.TrimSpace(userInput)
	historyBlock, historyMeta := s.buildPMHistoryBlock(store, sessionID)
	var b strings.Builder
	b.WriteString("event_type: user_input\n")
	b.WriteString("user_input:\n")
	if input == "" {
		b.WriteString("(none)")
	} else {
		b.WriteString(input)
	}
	b.WriteString("\n\nconversation_history:\n")
	if strings.TrimSpace(historyBlock) == "" {
		b.WriteString("(none)")
	} else {
		b.WriteString(strings.TrimSpace(historyBlock))
	}
	return strings.TrimSpace(b.String()), historyMeta
}

func (s *Server) buildPMHistoryBlock(store *projectstate.Store, sessionID string) (string, TaskPromptHistoryMeta) {
	if store == nil {
		return "", TaskPromptHistoryMeta{}
	}
	msgs, err := store.ListPMMessages(strings.TrimSpace(sessionID), 400)
	if err != nil {
		return "", TaskPromptHistoryMeta{}
	}
	taskLike := make([]projectstate.TaskMessageRecord, 0, len(msgs))
	for _, msg := range msgs {
		taskLike = append(taskLike, projectstate.TaskMessageRecord{
			ID:      msg.ID,
			Role:    strings.TrimSpace(msg.Role),
			Content: strings.TrimSpace(msg.Content),
		})
	}
	return buildTaskPromptHistory(taskLike, defaultTaskPromptHistoryOptions())
}
