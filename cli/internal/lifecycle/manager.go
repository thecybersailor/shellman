package lifecycle

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
)

type runJob struct {
	name string
	run  func(context.Context) error
}

type shutdownJob struct {
	name string
	run  func(context.Context) error
}

type Manager struct {
	mu           sync.Mutex
	runJobs      []runJob
	shutdownJobs []shutdownJob
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) AddRun(name string, fn func(context.Context) error) {
	if fn == nil {
		return
	}
	m.mu.Lock()
	m.runJobs = append(m.runJobs, runJob{name: name, run: fn})
	m.mu.Unlock()
}

func (m *Manager) AddShutdown(name string, fn func(context.Context) error) {
	if fn == nil {
		return
	}
	m.mu.Lock()
	m.shutdownJobs = append(m.shutdownJobs, shutdownJob{name: name, run: fn})
	m.mu.Unlock()
}

func (m *Manager) StartAndWait(parent context.Context, sig ...os.Signal) error {
	ctx := parent
	stopSignal := func() {}
	if len(sig) > 0 {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(parent, sig...)
		stopSignal = stop
	}
	defer stopSignal()

	runCtx, cancelRuns := context.WithCancel(ctx)
	defer cancelRuns()

	runJobs := m.snapshotRunJobs()
	shutdownJobs := m.snapshotShutdownJobs()

	errCh := make(chan error, len(runJobs))
	var wg sync.WaitGroup
	for _, job := range runJobs {
		job := job
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := job.run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- err
				cancelRuns()
			}
		}()
	}

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		cancelRuns()
	case err := <-errCh:
		runErr = err
		cancelRuns()
	case <-doneCh:
	}

	<-doneCh

	var shutdownErr error
	for _, job := range shutdownJobs {
		if err := job.run(context.Background()); err != nil && !errors.Is(err, context.Canceled) {
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	return errors.Join(runErr, shutdownErr)
}

func (m *Manager) snapshotRunJobs() []runJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]runJob, len(m.runJobs))
	copy(out, m.runJobs)
	return out
}

func (m *Manager) snapshotShutdownJobs() []shutdownJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]shutdownJob, len(m.shutdownJobs))
	copy(out, m.shutdownJobs)
	return out
}
