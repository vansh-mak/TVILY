package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// decodeModelJSON accepts a JSON object even when a model wraps it in a
// Markdown code fence or adds a short preamble or closing note.
func decodeModelJSON(output string, target any) error {
	start := strings.IndexByte(output, '{')
	if start < 0 {
		return fmt.Errorf("response contains no JSON object")
	}

	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(output); index++ {
		char := output[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch char {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return json.Unmarshal([]byte(output[start:index+1]), target)
			}
		}
	}
	return fmt.Errorf("response contains an incomplete JSON object")
}
