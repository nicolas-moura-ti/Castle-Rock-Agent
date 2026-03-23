package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveEvent(t *testing.T) {
	// Use an in-memory database: fast and clean for unit tests.
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

	// Attempt to save with the already canceled context.
	// The log MUST be saved because of the inner use of context.WithoutCancel.
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

func TestNewSQLiteStore_PersistentFile(t *testing.T) {
	// Integration test: validates if the database creates the file physically.
	tempDir := t.TempDir()
	dbPath := tempDir + "/test.db"

	store, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	store.SaveEvent(ctx, "start", "test-persistent", "Container started")

	var events []EventRecord
	require.Eventually(t, func() bool {
		events, err = store.GetRecent(10)
		return err == nil && len(events) > 0
	}, 2*time.Second, 50*time.Millisecond, "Event should be saved in persistent file")

	assert.Len(t, events, 1)
	assert.Equal(t, "test-persistent", events[0].Container)
}

func TestNewSQLiteStore_ErrorHandling(t *testing.T) {
	// Attempts to open a directory instead of a database file (should fail).
	tempDir := t.TempDir()

	store, err := NewSQLiteStore(tempDir)
	assert.Error(t, err, "Should fail when given a directory path instead of a file path")
	assert.Nil(t, store)
}