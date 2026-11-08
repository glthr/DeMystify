package parser

import (
	"regexp"
	"strings"
)

type handlerDefinition struct {
	name, kind, parameter string
	start, end            int
	body                  []ScriptLine
}

var handlerDeclaration = regexp.MustCompile(`(?i)^(on|function)\s+([a-z][a-z0-9_]*)(?:\s+(.*))?$`)

func activeCommand(line ScriptLine) string {
	command, disabled := normalizeCommand(line.Line)
	if disabled || line.IsDisabled {
		return ""
	}
	return strings.TrimSpace(command)
}

func scriptHandlers(script HyperTalk) []handlerDefinition {
	var handlers []handlerDefinition
	for i, line := range script.lines {
		m := handlerDeclaration.FindStringSubmatch(activeCommand(line))
		if m == nil {
			continue
		}
		for j := i + 1; j < len(script.lines); j++ {
			command := activeCommand(script.lines[j])
			if handlerDeclaration.MatchString(command) {
				break
			}
			fields := strings.Fields(command)
			if len(fields) == 2 && strings.EqualFold(fields[0], "end") && strings.EqualFold(fields[1], m[2]) {
				handlers = append(handlers, handlerDefinition{strings.ToLower(m[2]), strings.ToLower(m[1]), strings.TrimSpace(m[3]), i, j, script.lines[i+1 : j]})
				break
			}
		}
	}
	return handlers
}

func declaredHandlers(scripts []HyperTalk) map[string]int {
	names := make(map[string]int)
	for _, script := range scripts {
		for _, line := range script.lines {
			if m := handlerDeclaration.FindStringSubmatch(activeCommand(line)); m != nil {
				names[strings.ToLower(m[2])]++
			}
		}
	}
	return names
}

func shadowsHandlerPrimitive(names map[string]int) bool {
	return names["go"] != 0 || names["push"] != 0 || names["pop"] != 0 || names["put"] != 0
}
