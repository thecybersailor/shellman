package application

import "context"

// StartOptions defines unified startup options for local/turn runtime.
type StartOptions struct {
	Mode          string
	ConfigDir     string
	DBDSN         string
	LocalHost     string
	LocalPort     int
	WorkerBaseURL string
	TmuxSocket    string
	WebUI         WebUIOptions
	Hooks         Hooks
}

type WebUIOptions struct {
	Mode        string
	DevProxyURL string
	DistDir     string
}

type Hooks struct {
	Run          func(context.Context) error
	Shutdown     func(context.Context) error
	BootstrapTag string
	LocalAPIURL  string
}
