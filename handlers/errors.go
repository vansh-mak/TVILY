package handlers

import (
	"os"
	"strings"
)

func redactProviderToken(message string) string {
	for _, variable := range []string{"OPENROUTER_API_KEY"} {
		if token := strings.TrimSpace(os.Getenv(variable)); token != "" {
			message = strings.ReplaceAll(message, token, "[redacted]")
		}
	}
	return message
}
