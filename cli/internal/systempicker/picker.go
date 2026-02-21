package systempicker

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

func buildPickCommand(goos string) (string, []string) {
	switch goos {
	case "darwin":
		return "osascript", []string{"-e", `POSIX path of (choose folder with prompt "Select project root")`}
	case "linux":
		return "zenity", []string{"--file-selection", "--directory", "--title=Select project root"}
	case "windows":
		return "powershell", []string{
			"-NoProfile",
			"-Command",
			"Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.FolderBrowserDialog; if($d.ShowDialog() -eq 'OK'){Write-Output $d.SelectedPath}",
		}
	default:
		return "", nil
	}
}

func PickDirectory() (string, error) {
	cmd, args := buildPickCommand(runtime.GOOS)
	if cmd == "" {
		return "", errors.New("directory picker unsupported on this platform")
	}
	out, err := exec.Command(cmd, args...).Output()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", errors.New("empty directory selection")
	}
	return path, nil
}
