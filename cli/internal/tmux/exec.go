package tmux

import (
	"fmt"
	"os/exec"
	"strings"
)

type Exec interface {
	Output(name string, args ...string) ([]byte, error)
	Run(name string, args ...string) error
}

type RealExec struct{}

func (r *RealExec) Output(name string, args ...string) ([]byte, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return out, fmt.Errorf("%w: %s", err, msg)
		}
		return out, err
	}
	return out, nil
}

func (r *RealExec) Run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}
