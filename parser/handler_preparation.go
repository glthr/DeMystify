package parser

func (p *Parser) prepareStackHandlers(stack *HyperCardStack, dir string) {
	scripts, verified := p.backgroundScripts(stack, dir)
	background := declaredHandlers(scripts)
	declarations := declaredHandlers(stack.Script)
	if !verified || shadowsHandlerPrimitive(background) || shadowsHandlerPrimitive(declarations) {
		return
	}
	summaries := make(map[string]stackHandlerSummary)
	for _, script := range stack.Script {
		for _, handler := range scriptHandlers(script) {
			id, parameters, ok := summarizeHandler(handler)
			if !ok || declarations[handler.name] != 1 || background[handler.name] != 0 {
				continue
			}
			target, err := p.GetCardByStackAndID(stack.Name, id)
			if err == nil && !shadowsHandlerPrimitive(declaredHandlers(target.Scripts)) {
				summaries[handler.name] = stackHandlerSummary{parameters, target}
			}
		}
	}
	if p.stackHandlers == nil {
		p.stackHandlers = make(map[*HyperCardStack]map[string]stackHandlerSummary)
	}
	p.stackHandlers[stack] = summaries
	if len(summaries) != 0 {
		stack.IsPushStack = true
	}
}

func (p *Parser) withoutHandlerDefinitions(stack *HyperCardStack, script HyperTalk) HyperTalk {
	skip := make(map[int]bool)
	for _, handler := range scriptHandlers(script) {
		if _, ok := p.stackHandlers[stack][handler.name]; ok {
			for i := handler.start; i <= handler.end; i++ {
				skip[i] = true
			}
		}
	}
	var filtered HyperTalk
	for i, line := range script.lines {
		if !skip[i] {
			filtered.lines = append(filtered.lines, line)
		}
	}
	return filtered
}
