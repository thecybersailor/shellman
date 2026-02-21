package progdetector

import "strings"

func MatchProgramInCommand(currentCommand, programName string) bool {
	cmd := strings.ToLower(strings.TrimSpace(currentCommand))
	name := strings.ToLower(strings.TrimSpace(programName))
	if cmd == "" || name == "" {
		return false
	}
	parts := strings.Fields(cmd)
	if len(parts) > 0 && parts[0] == name {
		return true
	}
	return strings.Contains(cmd, name+" (") ||
		strings.Contains(cmd, "("+name+")") ||
		strings.Contains(cmd, "/"+name) ||
		strings.Contains(cmd, `\`+name)
}
