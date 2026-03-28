package triage

import (
	"strings"
)

type Command struct {
	Name string
	Args []string
}

// ParseCommand extracts a @botname command from a comment body.
// It finds the first occurrence of @botname and parses the rest of that line.
func ParseCommand(body, botName string) (Command, bool) {
	mention := "@" + botName
	lines := strings.Split(body, "\n")

	for _, line := range lines {
		idx := strings.Index(line, mention)
		if idx == -1 {
			continue
		}

		after := strings.TrimSpace(line[idx+len(mention):])
		if after == "" {
			continue
		}

		parts := strings.Fields(after)
		if len(parts) == 0 {
			continue
		}

		cmd := Command{Name: parts[0]}
		if len(parts) > 1 {
			cmd.Args = parts[1:]
		}
		return cmd, true
	}

	return Command{}, false
}
