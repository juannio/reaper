package reaper

import (
	"sync"
	"time"
)

type RestartPolicy int
type State string

const (
	Never RestartPolicy = iota
	OnFailure
	Always
)

const (
	StateStarting   State = "starting"
	StateHealthy    State = "healthy"
	StateRestarting State = "restarting"
)

type Process struct {
	Name        string
	Command     []string
	Restart     RestartPolicy
	Backoff     time.Duration
	HealthCheck HealthCheck
	ProcStatus  *ProcStatus
}

type ProcStatus struct {
	mu             sync.Mutex
	Name           string
	State          State
	Retries        int
	CurrentBackoff time.Duration
	StartedAt      time.Time
}
