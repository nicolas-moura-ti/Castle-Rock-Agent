package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveEvent(t *testing.T) {
	// Use an in-memory database, avoiding file cleanup overhead
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Call SaveEvent
	ctx := context.Background()
	store.SaveEvent(ctx, "start", "test-container", "Container started")

	// Wait for the async operation to complete or timeout
	// This is a common pattern for testing async operations without flaky sleeps
	// We'll poll the database until we find the record or hit a timeout

	var events []EventRecord
	require.Eventually(t, func() bool {
		events, err = store.GetRecent(10)
		return err == nil && len(events) > 0
	}, 2*time.Second, 50*time.Millisecond, "Event should be saved asynchronously")

	// Verify the event details
	assert.Len(t, events, 1)
	if len(events) > 0 {
		event := events[0]
		assert.Equal(t, "event", event.Type)
		assert.Equal(t, "start", event.Action)
		assert.Equal(t, "test-container", event.Container)
		assert.Equal(t, "Container started", event.Message)
		// Check that timestamp is recent
		assert.WithinDuration(t, time.Now(), event.Timestamp, 5*time.Second)
	}
}

func TestSaveWithCanceledContext(t *testing.T) {
	// Use an in-memory database shared cache, avoiding file cleanup overhead
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Create a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Ensure the context is actually canceled before proceeding
	require.ErrorIs(t, ctx.Err(), context.Canceled)

	// Call SaveEvent with the canceled context
	store.SaveEvent(ctx, "stop", "test-container-2", "Container stopped")

	// Wait for the async operation to complete or timeout
	// Even though the input context was canceled, the inner `save()` implementation
	// drops the input context and uses `context.WithoutCancel` (or `context.Background`)
	// to allow the database write to complete successfully.
	var events []EventRecord
	require.Eventually(t, func() bool {
		events, err = store.GetRecent(10)
		return err == nil && len(events) > 0
	}, 2*time.Second, 50*time.Millisecond, "Event should be saved asynchronously despite canceled context")

	// Verify the event details
	assert.Len(t, events, 1)
	if len(events) > 0 {
		event := events[0]
		assert.Equal(t, "event", event.Type)
		assert.Equal(t, "stop", event.Action)
		assert.Equal(t, "test-container-2", event.Container)
		assert.Equal(t, "Container stopped", event.Message)
		// Check that timestamp is recent
		assert.WithinDuration(t, time.Now(), event.Timestamp, 5*time.Second)
	}
}

func TestSaveAlert(t *testing.T) {
	// Use an in-memory database shared cache, avoiding file cleanup overhead
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Call SaveAlert
	ctx := context.Background()
	store.SaveAlert(ctx, "critical", "vuln-container", "Security vulnerability detected")

	// Wait for the async operation to complete or timeout
	var events []EventRecord
	require.Eventually(t, func() bool {
		events, err = store.GetRecent(10)
		return err == nil && len(events) > 0
	}, 2*time.Second, 50*time.Millisecond, "Alert should be saved asynchronously")

	// Verify the alert details
	assert.Len(t, events, 1)
	if len(events) > 0 {
		event := events[0]
		assert.Equal(t, "alert", event.Type)
		assert.Equal(t, "critical", event.Action) // Severity is saved in the Action column
		assert.Equal(t, "vuln-container", event.Container)
		assert.Equal(t, "Security vulnerability detected", event.Message)
		// Check that timestamp is recent
		assert.WithinDuration(t, time.Now(), event.Timestamp, 5*time.Second)
	}
}
