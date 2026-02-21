package application

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
}

type WebUIOptions struct {
	Mode        string
	DevProxyURL string
	DistDir     string
}
