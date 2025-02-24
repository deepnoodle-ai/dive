package dive

import "strings"

func parseStructuredResponse(responseText string) (string, string, string) {
	var response, thinking, reportedStatus string

	// Split on <think> tag
	if strings.Contains(responseText, "<think>") {
		parts := strings.Split(responseText, "<think>")
		if len(parts) > 1 {
			// Find the end of think section
			thinkParts := strings.Split(parts[1], "</think>")
			if len(thinkParts) > 1 {
				thinking = strings.TrimSpace(thinkParts[0])
				response = strings.TrimSpace(thinkParts[1])
			}
		}
	} else {
		response = responseText
	}

	// Extract status if present
	if strings.Contains(response, "<status>") {
		parts := strings.Split(response, "<status>")
		if len(parts) > 1 {
			statusParts := strings.Split(parts[1], "</status>")
			if len(statusParts) > 1 {
				reportedStatus = strings.TrimSpace(statusParts[0])
				response = strings.TrimSpace(parts[0])
			}
		}
	}

	return response, thinking, reportedStatus
}
