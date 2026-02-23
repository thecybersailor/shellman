package localapi

import "strings"

type PMToolPolicy struct {
	DenyWrite    bool
	DenyNetwork  bool
	DenyPlanning bool
}

type PMToolProfile struct {
	tools []string
}

func buildPMToolProfileCodexParity() PMToolProfile {
	return PMToolProfile{tools: []string{
		"exec_command",
		"write_stdin",
		"apply_patch",
		"update_plan",
		"view_image",
		"request_user_input",
		"multi_tool_use.parallel",
		"web.search_query",
		"web.open",
		"web.click",
		"web.find",
		"web.screenshot",
		"web.image_query",
		"web.finance",
		"web.weather",
		"web.sports",
		"web.time",
	}}
}

func (p PMToolProfile) ToolNames() []string {
	out := make([]string, 0, len(p.tools))
	seen := map[string]struct{}{}
	for _, item := range p.tools {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func (p PMToolProfile) ResolveAllowedTools(policy PMToolPolicy) []string {
	base := p.ToolNames()
	if !policy.DenyWrite && !policy.DenyNetwork && !policy.DenyPlanning {
		return base
	}
	out := make([]string, 0, len(base))
	for _, name := range base {
		if policy.DenyWrite {
			switch name {
			case "apply_patch", "write_stdin":
				continue
			}
		}
		if policy.DenyNetwork {
			if strings.HasPrefix(name, "web.") {
				continue
			}
		}
		if policy.DenyPlanning {
			switch name {
			case "update_plan", "request_user_input":
				continue
			}
		}
		out = append(out, name)
	}
	return out
}
