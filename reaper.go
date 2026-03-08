package reaper

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"time"
)

type Reaper struct {
	// Process structure
	process []*Process
}

func New() *Reaper {
	return &Reaper{}
}

func (r *Reaper) Add(p Process) {
	// Add processes
	r.process = append(r.process, &p)
}

func (r *Reaper) Start(ctx context.Context) {
	for _, p := range r.process {
		// Launch go routine for each process
		go r.watch(ctx, p)
	}
}

func (r *Reaper) Status() []*Process {
	return r.process
}

func (s *ProcStatus) update(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

func (s *ProcStatus) Snapshot() (State, int, time.Duration, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.State, s.Retries, s.CurrentBackoff, s.StartedAt
}

type HealthCheck interface {
	Check(ctx context.Context) error
}

type TCPCheck struct {
	Addr string
}

func (t *TCPCheck) Check(ctx context.Context) error {
	conn, err := net.Dial("tcp", t.Addr)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

type HTTPCheck struct {
	URL string
}

func (h *HTTPCheck) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.URL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy status code: %d", resp.StatusCode)
	}
	return nil
}

type ExecCheck struct {
	Command []string
}

func (e *ExecCheck) Check(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, e.Command[0], e.Command[1:]...)
	_, err := cmd.Output()
	if err != nil {
		return (err)
	}
	return nil
}

// Go routine that supervises process
func (r *Reaper) watch(ctx context.Context, p *Process) {
	status := &ProcStatus{
		Name:           p.Name,
		CurrentBackoff: p.Backoff,
	}
	p.ProcStatus = status
	currentBackoff := p.Backoff
	retries := 0
	for {
		slog.Info("starting process", "name", p.Name)

		status.update(func() {
			status.State = StateStarting
		})

		cmd := exec.CommandContext(ctx, p.Command[0], p.Command[1:]...)

		if err := cmd.Start(); err != nil {
			slog.Error("failed to start", "name", p.Name, "error", err)
			return
		}

		slog.Info("waiting for process to be reay", "name", p.Name)
		if err := waitUntilReady(ctx, p.HealthCheck); err != nil {
			slog.Info("never became ready", "name", p.Name, "error", err)
			return
		}

		slog.Info("process is ready", "name", p.Name)

		if ctx.Err() != nil {
			slog.Warn("context cancelled, stopping", "name", p.Name)
			return
		}

		// Here is where process ends/exit
		// Record process uptime
		startTime := time.Now()
		status.update(func() {
			status.State = StateHealthy
			status.StartedAt = startTime
		})
		err := cmd.Wait()

		// ------>> Continues <<--------
		upTime := time.Since(startTime)

		// If proc stays healty for 45 secs, restart exponential backoff
		if upTime > time.Second*45 {
			currentBackoff = p.Backoff
			retries = 0
			status.update(func() {
				status.CurrentBackoff = currentBackoff
				status.Retries = retries
			})
		}

		if err != nil {
			slog.Error("process exited with error", "name", p.Name, "error", err)
		} else {
			slog.Info("process exited cleanly", "name", p.Name)
		}

		// After process exits, we need to determine if we will restart it or not, based on <RestartPolicy>
		switch p.Restart {
		case Never:
			slog.Info("process will not be restarted", "name", p.Name)
			return
		case OnFailure:
			// If porcess exit is not due error
			if err == nil {
				slog.Info("process exited cleanly, not restarting", "name", p.Name)
				return
			}
		case Always:
		}

		status.update(func() {
			status.State = StateRestarting
			status.Retries = retries
			status.CurrentBackoff = currentBackoff
		})

		retries++
		slog.Warn("restarting process", "name", p.Name, "backoff", currentBackoff.String(), "retries", retries)
		time.Sleep(currentBackoff)
		currentBackoff = min(currentBackoff*2, 30*time.Second)
	}
}

func waitUntilReady(ctx context.Context, check HealthCheck) error {
	for {
		if err := check.Check(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
