package prune

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/storage"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoPruner_Start(t *testing.T) {
	// Setup client (dummy client for this test as we don't trigger prune)
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:2375") // prevent reaching real docker daemon
	cli, _ := docker.NewClient()

	pruner := NewAutoPruner(cli, nil, 80.0)

	checkCalled := make(chan struct{}, 1)

	// Explicitly mock the diskCheckFunc to return 0% usage to prevent any pruning logic
	// and signal that it was called.
	pruner.diskCheckFunc = func(ctx context.Context, path string) (*disk.UsageStat, error) {
		select {
		case checkCalled <- struct{}{}:
		default:
		}
		return &disk.UsageStat{UsedPercent: 0.0}, nil
	}

	// Reduce check period drastically to avoid waiting in tests
	pruner.checkPeriod = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())

	// Channel to signal that Start has returned
	done := make(chan struct{})
	go func() {
		pruner.Start(ctx)
		close(done)
	}()

	// Wait for the loop to iterate at least once
	select {
	case <-checkCalled:
		// success, checkAndPrune was executed
	case <-time.After(1 * time.Second):
		t.Fatal("checkAndPrune was not called within timeout")
	}

	// Cancel context to stop the goroutine
	cancel()

	// Wait for goroutine to exit (with a reasonable timeout to prevent deadlocks)
	select {
	case <-done:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("Start() did not return after context cancellation")
	}
}

func TestAutoPruner_CheckAndPrune(t *testing.T) {
	// Setup in-memory sqlite store
	store, err := storage.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Mock Docker Daemon
	var mu sync.Mutex
	var pruneCalls int

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" {
			w.Header().Set("API-Version", "1.43")
			w.Write([]byte("OK"))
			return
		}

		// Mock the prune endpoints used by PruneSystem
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/prune") {
			mu.Lock()
			pruneCalls++
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"SpaceReclaimed": 1048576}`)) // 1MB reclaimed
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	// Redirect Docker client to the mock server
	t.Setenv("DOCKER_HOST", mockServer.URL)
	cli, err := docker.NewClient()
	require.NoError(t, err)
	defer cli.Close()

	t.Run("Prune Triggered (Exceeds Threshold)", func(t *testing.T) {
		mu.Lock()
		pruneCalls = 0
		mu.Unlock()

		pruner := NewAutoPruner(cli, store, 80.0)

		// Set mock disk check function to return 90% usage
		pruner.diskCheckFunc = func(ctx context.Context, path string) (*disk.UsageStat, error) {
			return &disk.UsageStat{UsedPercent: 90.0}, nil
		}

		// Ensure it thinks the last prune was way in the past
		pruner.lastPrune = time.Now().Add(-2 * time.Hour)

		pruner.checkAndPrune(context.Background())

		mu.Lock()
		calls := pruneCalls
		mu.Unlock()

		assert.Greater(t, calls, 0, "PruneSystem should have been called at least once")

		// Verify event was saved (give async save a moment to complete)
		time.Sleep(50 * time.Millisecond)
		events, err := store.GetRecent(10)
		require.NoError(t, err)
		assert.NotEmpty(t, events, "Event should be saved in DB")

		found := false
		for _, e := range events {
			if e.Action == "prune" && strings.Contains(e.Message, "AutoPrune triggered") {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected AutoPrune event in DB")
	})

	t.Run("Prune Not Triggered (Below Threshold)", func(t *testing.T) {
		mu.Lock()
		pruneCalls = 0
		mu.Unlock()

		pruner := NewAutoPruner(cli, store, 80.0)

		// Set mock disk check function to return 50% usage
		pruner.diskCheckFunc = func(ctx context.Context, path string) (*disk.UsageStat, error) {
			return &disk.UsageStat{UsedPercent: 50.0}, nil
		}

		pruner.lastPrune = time.Now().Add(-2 * time.Hour)

		pruner.checkAndPrune(context.Background())

		mu.Lock()
		calls := pruneCalls
		mu.Unlock()

		assert.Equal(t, 0, calls, "PruneSystem should NOT have been called")
	})

	t.Run("Prune Throttled (Called Recently)", func(t *testing.T) {
		mu.Lock()
		pruneCalls = 0
		mu.Unlock()

		pruner := NewAutoPruner(cli, store, 80.0)

		// Set mock disk check function to return 90% usage (should trigger if not throttled)
		pruner.diskCheckFunc = func(ctx context.Context, path string) (*disk.UsageStat, error) {
			return &disk.UsageStat{UsedPercent: 90.0}, nil
		}

		// Set lastPrune to just a minute ago (throttled is 1h)
		pruner.lastPrune = time.Now().Add(-1 * time.Minute)

		pruner.checkAndPrune(context.Background())

		mu.Lock()
		calls := pruneCalls
		mu.Unlock()

		assert.Equal(t, 0, calls, "PruneSystem should NOT have been called because of throttling")
	})

	t.Run("Disk Check Error (Does Not Prune)", func(t *testing.T) {
		mu.Lock()
		pruneCalls = 0
		mu.Unlock()

		pruner := NewAutoPruner(cli, store, 80.0)

		// Set mock disk check function to return error
		pruner.diskCheckFunc = func(ctx context.Context, path string) (*disk.UsageStat, error) {
			return nil, fmt.Errorf("simulated disk error")
		}

		pruner.lastPrune = time.Now().Add(-2 * time.Hour)

		pruner.checkAndPrune(context.Background())

		mu.Lock()
		calls := pruneCalls
		mu.Unlock()

		assert.Equal(t, 0, calls, "PruneSystem should NOT have been called on disk error")
	})
}
