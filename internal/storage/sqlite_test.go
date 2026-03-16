package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveEvent(t *testing.T) {
	// Create a temporary file for the database
	tmpfile, err := os.CreateTemp("", "testdb-*.sqlite")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	store, err := NewSQLiteStore(tmpfile.Name())
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
