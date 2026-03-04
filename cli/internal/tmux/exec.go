package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type Exec interface {
	Output(name string, args ...string) ([]byte, error)
	Run(name string, args ...string) error
}

type RealExec struct {
	mu       sync.Mutex
	tmuxPath string
	pathInit bool
}

func (r *RealExec) resolveExecutable(name string) (string, error) {
	if strings.TrimSpace(name) != "tmux" {
		return name, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pathInit {
		if r.tmuxPath != "" {
			return r.tmuxPath, nil
		}
		return "", exec.ErrNotFound
	}
	r.pathInit = true
	path, err := exec.LookPath("tmux")
	if err != nil {
		r.tmuxPath = ""
		return "", err
	}
	r.tmuxPath = path
	return r.tmuxPath, nil
}

func (r *RealExec) Output(name string, args ...string) ([]byte, error) {
	resolvedName, err := r.resolveExecutable(name)
	if err != nil {
		return nil, err
	}
	out, err := exec.Command(resolvedName, args...).CombinedOutput()
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
	resolvedName, err := r.resolveExecutable(name)
	if err != nil {
		return err
	}
	out, err := exec.Command(resolvedName, args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}
