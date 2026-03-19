package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveEvent(t *testing.T) {
	// Use an in-memory database: fast and clean.
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	store.SaveEvent(ctx, "start", "test-container", "Container started")

	// Poll to verify asynchronous write
	var events []EventRecord
	require.Eventually(t, func() bool {
		events, err = store.GetRecent(10)
		return err == nil && len(events) > 0
	}, 2*time.Second, 50*time.Millisecond, "Event should be saved asynchronously")

	assert.Len(t, events, 1)
	if len(events) > 0 {
		event := events[0]
		assert.Equal(t, "event", event.Type)
		assert.Equal(t, "start", event.Action)
		assert.Equal(t, "test-container", event.Container)
		assert.Equal(t, "Container started", event.Message)
		assert.WithinDuration(t, time.Now(), event.Timestamp, 5*time.Second)
	}
}

// TestSaveWithCanceledContext validates that the use of context.WithoutCancel is working.
// The log MUST be saved even if the original context has been canceled.
func TestSaveWithCanceledContext(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Create a context and cancel it immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() 

	require.ErrorIs(t, ctx.Err(), context.Canceled)

	// Attempt to save with the already canceled context
	store.SaveEvent(ctx, "stop", "test-container-2", "Container stopped")

	var events []EventRecord
	require.Eventually(t, func() bool {
		events, err = store.GetRecent(10)
		return err == nil && len(events) > 0
	}, 2*time.Second, 50*time.Millisecond, "Event should be saved despite canceled context")

	assert.Len(t, events, 1)
	if len(events) > 0 {
		assert.Equal(t, "test-container-2", events[0].Container)
	}
}

func TestSaveAlert(t *testing.T) {
	// Standardized to use :memory: instead of temporary files
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	store.SaveAlert(ctx, "critical", "nginx-web", "High CPU usage detected")

	var events []EventRecord
	require.Eventually(t, func() bool {
		events, err = store.GetRecent(10)
		return err == nil && len(events) > 0
	}, 2*time.Second, 50*time.Millisecond, "Alert should be saved asynchronously")

	assert.Len(t, events, 1)
	if len(events) > 0 {
		event := events[0]
		assert.Equal(t, "alert", event.Type)
		assert.Equal(t, "critical", event.Action)
		assert.Equal(t, "nginx-web", event.Container)
		assert.Contains(t, event.Message, "High CPU")
		assert.WithinDuration(t, time.Now(), event.Timestamp, 5*time.Second)
	}
}