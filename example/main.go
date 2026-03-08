package example

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/juannio/reaper"
	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client

func main() {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Listen for SIGTERM, SIGINT
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigs
		log.Printf("received signal: %s shutting down", sig)
		cancel()
	}()

	// Create reaper
	r := reaper.New()

	// Add redis as supervised process
	r.Add(reaper.Process{
		Name:        "redis",
		Command:     []string{"redis-server"},
		Restart:     reaper.Always,
		Backoff:     2 * time.Second,
		HealthCheck: &reaper.TCPCheck{Addr: "localhost:6379"}, // HealtCheck type TCP
	})

	// Launches reaper, starts process
	r.Start(ctx)

	// Redis instance
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	http.HandleFunc("/data", handleData)
	http.HandleFunc("/status", func(w http.ResponseWriter, req *http.Request) {
		for _, p := range r.Status() {
			state, retries, backoff, startedAt := p.ProcStatus.Snapshot()
			uptime := time.Since(startedAt)
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "%s status: state=%s uptime=%s retries=%d\n backoff=%s\n", p.Name, state, uptime, retries, backoff.String())
		}

	})

	go func() {
		log.Println("Server starting on :8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Printf("http server error: %v", err)
		}
	}()

	// Block till we call cancel()
	<-ctx.Done()
	log.Println("shutdown complete")
}

func handleData(w http.ResponseWriter, r *http.Request) {

	val, err := rdb.Get(r.Context(), "message").Result()
	if err != nil {
		http.Error(w, "failedd to get data from Redis", http.StatusInternalServerError)
		return
	}

	w.Write([]byte(val))
}
