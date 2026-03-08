# reaper

A lightweight process supervision library for Go. Embed it in your application to automatically manage and restart child processes — no external tools required.

> ⚠️ **Early stage practice project.**     Hands-on learning exercise to explore Go systems programming concepts. Features are incomplete, edge cases are unhandled, the API may change at any time, and several enhancements are still in progress. Contributions, feedback, and ideas are welcome.

## The Problem

When your application depends on external processes (databases, workers, sidecars), those processes can crash. Without supervision, they stay dead until someone manually restarts them.

```
Redis crashes at 3am
    └── your app starts returning 500s
    └── someone gets paged
    └── someone manually restarts Redis
```

Reaper solves this by supervising child processes from within your application.

## Installation

```bash
go get github.com/juannio/reaper
```

## Usage

```go
package main

import (
    "context"
    "time"
    "github.com/juannio/reaper/reaper"
)

func main() {
    ctx := context.Background()

    r := reaper.New()
    r.Add(reaper.Process{
        Name:        "redis",
        Command:     []string{"redis-server"},
        Restart:     reaper.Always,
        Backoff:     2 * time.Second,
        HealthCheck: &reaper.TCPCheck{Addr: "localhost:6379"},
    })

    r.Start(ctx)
    // redis-server is now supervised — reaper will restart it if it crashes
}
```

## Restart Policies

| Policy      | Behavior                                        |
| ----------- | ----------------------------------------------- |
| `Always`    | Restart regardless of exit code                 |
| `OnFailure` | Restart only if process crashed (non-zero exit) |
| `Never`     | Do not restart                                  |

## Health Checks

Reaper waits until a process is actually ready before declaring it healthy.

```go
// TCP — try to open a connection
&reaper.TCPCheck{Addr: "localhost:6379"}

// HTTP — expect a 200 response
&reaper.HTTPCheck{URL: "http://localhost:8080/healthz"}

// Exec — run a command, expect exit code 0
&reaper.ExecCheck{Command: []string{"redis-cli", "ping"}}
```

You can also implement your own by satisfying the `HealthCheck` interface:

```go
type HealthCheck interface {
    Check(ctx context.Context) error
}
```

## Exponential Backoff

Reaper doubles the wait time between restarts to avoid hammering a repeatedly crashing process. The backoff resets automatically after the process stays healthy for 45 seconds.

```
crash #1 → wait 2s
crash #2 → wait 4s
crash #3 → wait 8s
crash #4 → wait 16s
crash #5 → wait 30s (capped)
```

## Status API

Inspect the current state of all supervised processes:

```go
for _, p := range r.Status() {
    state, retries, backoff, startedAt := p.ProcStatus.Snapshot()
    // state: "starting" | "healthy" | "restarting"
}
```

## Graceful Shutdown

Pass a cancellable context to `Start()`. When the context is cancelled, reaper stops all supervised processes cleanly.

```go
ctx, cancel := context.WithCancel(context.Background())

sigs := make(chan os.Signal, 1)
signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

go func() {
    <-sigs
    cancel() // stops reaper and all child processes
}()

r.Start(ctx)
```

## Structured Logging

Reaper uses Go's `log/slog` for structured JSON logging. Set a JSON handler in your application to get machine-readable logs:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
slog.SetDefault(logger)
```

Output:

```json
{"time":"2026-03-07T19:02:15Z","level":"INFO","msg":"process is ready","name":"redis"}
{"time":"2026-03-07T19:02:28Z","level":"WARN","msg":"restarting process","name":"redis","backoff":"2s","retries":1}
```

## Roadmap

The following features are planned or in progress:

- [ ] Max retries — give up after N consecutive crashes
- [ ] Detached processes — processes that survive parent death
- [ ] Supervision trees — process dependencies and ordered startup
- [ ] Metrics — expose crash counts and uptime as Prometheus metrics
- [ ] Tests — unit and integration test coverage
- [ ] Published Go module

## License

MIT
