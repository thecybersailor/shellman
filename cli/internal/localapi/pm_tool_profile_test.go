package localapi

import "testing"

func TestPMToolProfile_CodexParityContainsRequiredTools(t *testing.T) {
	names := buildPMToolProfileCodexParity().ToolNames()
	mustContain := []string{
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
	}
	for _, name := range mustContain {
		if !containsToolName(names, name) {
			t.Fatalf("missing tool %s", name)
		}
	}
}

func TestPMToolProfile_ResolveAllowedToolsBySessionPolicy(t *testing.T) {
	profile := buildPMToolProfileCodexParity()
	allowed := profile.ResolveAllowedTools(PMToolPolicy{DenyWrite: true, DenyNetwork: true})
	if containsToolName(allowed, "apply_patch") {
		t.Fatal("apply_patch should be denied")
	}
	for _, name := range allowed {
		if len(name) >= 4 && name[:4] == "web." {
			t.Fatalf("web tool should be denied, got %s", name)
		}
	}
}

func containsToolName(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
