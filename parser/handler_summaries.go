package parser

import (
	"regexp"
	"strconv"
	"strings"
)

type stackHandlerSummary struct {
	parameters int
	target     *HyperCardCard
}

var (
	handlerIdentifier = regexp.MustCompile(`(?i)^[a-z][a-z0-9_]*$`)
	handlerAssignment = regexp.MustCompile(`(?i)^put\s+([a-z][a-z0-9_]*)\s+into\s+([a-z][a-z0-9_]*)$`)
	handlerLiteralGo  = regexp.MustCompile(`(?i)^go(?:\s+to)?\s+card\s+id\s+([0-9]+)$`)
)

// Summaries cover only declarations/parameter state assignment, one push and one literal go.
func summarizeHandler(handler handlerDefinition) (int, int, bool) {
	if handler.kind != "on" || handler.parameter != "" && !handlerIdentifier.MatchString(handler.parameter) {
		return 0, 0, false
	}
	parameter := strings.ToLower(handler.parameter)
	globals := make(map[string]bool)
	phase, target := 0, 0
	for _, line := range handler.body {
		command := activeCommand(line)
		if command == "" {
			continue
		}
		fields := strings.Fields(command)
		if phase == 0 && strings.EqualFold(fields[0], "global") {
			for _, name := range strings.Split(strings.Join(fields[1:], " "), ",") {
				name = strings.ToLower(strings.TrimSpace(name))
				if !handlerIdentifier.MatchString(name) || name == parameter {
					return 0, 0, false
				}
				globals[name] = true
			}
			continue
		}
		if m := handlerAssignment.FindStringSubmatch(command); phase == 0 && m != nil && parameter != "" && strings.EqualFold(m[1], parameter) && globals[strings.ToLower(m[2])] {
			continue
		}
		if phase == 0 && strings.EqualFold(strings.Join(fields, " "), "push card") {
			phase = 1
			continue
		}
		if m := handlerLiteralGo.FindStringSubmatch(command); phase == 1 && m != nil {
			id, err := strconv.Atoi(m[1])
			if err == nil && id > 0 {
				target, phase = id, 2
				continue
			}
		}
		return 0, 0, false
	}
	parameters := 0
	if parameter != "" {
		parameters = 1
	}
	return target, parameters, phase == 2
}
