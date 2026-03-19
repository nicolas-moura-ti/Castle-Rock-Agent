package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveEvent(t *testing.T) {
	// Uso de banco em memória: rápido e limpo.
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	store.SaveEvent(ctx, "start", "test-container", "Container started")

	// Poll para verificar a escrita assíncrona
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

// TestSaveWithCanceledContext valida se o uso de context.WithoutCancel está funcionando.
// O log DEVE ser salvo mesmo que o contexto original tenha sido cancelado.
func TestSaveWithCanceledContext(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Criamos um contexto e cancelamos ele imediatamente
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.ErrorIs(t, ctx.Err(), context.Canceled)

	// Tentamos salvar com o contexto já cancelado
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
