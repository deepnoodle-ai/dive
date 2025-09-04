package cli

import (
	"fmt"
	"strings"
)

// ConfirmAction prompts the user for confirmation with a standardized message
func ConfirmAction(action, target string) bool {
	fmt.Printf("❓ Are you sure you want to %s %s? [y/N]: ", action, target)
	var response string
	fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}
