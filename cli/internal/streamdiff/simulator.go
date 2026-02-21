package streamdiff

type Scenario string

const ScenarioFullscreenRedraw Scenario = "fullscreen_redraw"

type Frame struct {
	Target string
	Text   string
}

func BuildScenario(s Scenario) []Frame {
	if s != ScenarioFullscreenRedraw {
		return nil
	}
	return []Frame{
		{Target: "e2e:0.0", Text: "Header\nRow A ...\nFooter"},
		{Target: "e2e:0.0", Text: "Header\nRow B ...\nFooter"},
		{Target: "e2e:0.0", Text: "Header\nRow C ...\nFooter"},
		{Target: "e2e:0.0", Text: "Header\nRow D ...\nFooter"},
		{Target: "e2e:0.0", Text: "Header\nRow E ...\nFooter"},
		{Target: "e2e:0.0", Text: "Header\nRow F ...\nFooter"},
	}
}
