package agent

import (
	"strings"

	"github.com/diveagents/dive"
)

type StructuredResponse struct {
	Thinking          string
	Text              string
	StatusDescription string
}

func (sr StructuredResponse) Status() dive.TaskStatus {
	fields := strings.Fields(sr.StatusDescription)
	if len(fields) == 0 {
		return dive.TaskStatusInvalid
	}
	// Find the first matching status
	for _, field := range fields {
		value := strings.TrimPrefix(field, "\"")
		value = strings.TrimSuffix(value, "\"")
		switch value {
		case "active":
			return dive.TaskStatusActive
		case "paused":
			return dive.TaskStatusPaused
		case "completed":
			return dive.TaskStatusCompleted
		case "blocked":
			return dive.TaskStatusBlocked
		case "error":
			return dive.TaskStatusError
		}
	}
	return dive.TaskStatusInvalid
}

func ParseStructuredResponse(text string) StructuredResponse {
	var thinking, reportedStatus string
	workingText := text

	// Extract status if present
	statusStart := strings.Index(workingText, "<status>")
	statusEnd := strings.Index(workingText, "</status>")
	if statusStart != -1 && statusEnd != -1 && statusEnd > statusStart {
		reportedStatus = strings.TrimSpace(workingText[statusStart+8 : statusEnd])
		// Remove the status tag and its content
		workingText = workingText[:statusStart] + workingText[statusEnd+9:]
	}

	// Extract thinking if present
	thinkStart := strings.Index(workingText, "<think>")
	thinkEnd := strings.Index(workingText, "</think>")
	if thinkStart != -1 && thinkEnd != -1 && thinkEnd > thinkStart {
		thinking = strings.TrimSpace(workingText[thinkStart+7 : thinkEnd])
		// Remove the think tag and its content
		workingText = workingText[:thinkStart] + workingText[thinkEnd+8:]
	}

	// The response is whatever text remains, trimmed
	response := strings.TrimSpace(workingText)

	return StructuredResponse{
		Thinking:          thinking,
		Text:              response,
		StatusDescription: reportedStatus,
	}
}
