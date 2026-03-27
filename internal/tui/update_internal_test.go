package tui

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/nicolas-moura-ti/castle-rock-agent/internal/logger"
	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestModel_Update(t *testing.T) {
	m := Model{
		events: []EventLogEntry{},
		stats:  make(map[string]models.ContainerMetrics),
	}

	t.Run("tea.KeyMsg", func(t *testing.T) {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
		newM, _ := m.Update(msg)
		assert.NotNil(t, newM)
	})

	t.Run("tea.WindowSizeMsg", func(t *testing.T) {
		msg := tea.WindowSizeMsg{Width: 100, Height: 50}
		newM, cmd := m.Update(msg)
		assert.Nil(t, cmd)
		model := newM.(Model)
		assert.Equal(t, 100, model.width)
		assert.Equal(t, 50, model.height)
	})

	t.Run("containerListMsg", func(t *testing.T) {
		msg := containerListMsg{
			containers: []logger.ContainerDisplay{
				{ID: "c1", Name: "container-1"},
			},
		}
		newM, cmd := m.Update(msg)
		assert.Nil(t, cmd)
		model := newM.(Model)
		assert.Len(t, model.containers, 1)
		assert.Equal(t, "container-1", model.containers[0].Name)
	})

	t.Run("statsMsg", func(t *testing.T) {
		statsMap := map[string]models.ContainerMetrics{
			"c1": {CPUPercent: 10.0, MemoryPercent: 20.0},
		}
		msg := statsMsg{stats: statsMap}
		newM, cmd := m.Update(msg)
		assert.Nil(t, cmd)
		model := newM.(Model)
		assert.Equal(t, 10.0, model.stats["c1"].CPUPercent)
		assert.Equal(t, 20.0, model.stats["c1"].MemoryPercent)
	})

	t.Run("dockerEventMsg", func(t *testing.T) {
		event := docker.ContainerEvent{
			Time:          time.Now(),
			Action:        "start",
			ContainerID:   "c1",
			ContainerName: "container-1",
		}
		msg := dockerEventMsg{event: event}
		newM, cmd := m.Update(msg)
		assert.NotNil(t, cmd) // watchNextDockerEvent
		model := newM.(Model)
		assert.NotEmpty(t, model.events)
		assert.Equal(t, "start", model.events[0].Action)
	})

	t.Run("dockerErrorMsg", func(t *testing.T) {
		msg := dockerErrorMsg{err: fmt.Errorf("test error")}
		freshM := Model{events: []EventLogEntry{}}
		newFreshM, cmd := freshM.Update(msg)
		assert.Nil(t, cmd)
		model := newFreshM.(Model)
		assert.Len(t, model.events, 1)
		assert.Equal(t, "test error", model.events[0].Name)
		assert.Equal(t, "error", model.events[0].Action)
		assert.Equal(t, "❌", model.events[0].Icon)
	})

	t.Run("logLineMsg", func(t *testing.T) {
		logCh := make(chan string, 1)
		msg := logLineMsg{
			container: "container-1",
			line:      "log message",
			nextCh:    logCh,
		}
		newM, cmd := m.Update(msg)
		assert.NotNil(t, cmd) // waitForNextLog
		model := newM.(Model)
		assert.Len(t, model.logLines, 1)
		assert.Equal(t, "log message", model.logLines[0].Text)
		assert.Equal(t, "container-1", model.logLines[0].Container)
	})

	t.Run("actionResultMsg", func(t *testing.T) {
		freshM := Model{events: []EventLogEntry{}}
		msg := actionResultMsg{
			success:   true,
			action:    "stop",
			container: "container-1",
		}
		newM, cmd := freshM.Update(msg)
		assert.NotNil(t, cmd) // tea.Batch
		model := newM.(Model)
		assert.Len(t, model.events, 1)
		assert.Equal(t, "container-1", model.events[0].Name)
		assert.Equal(t, "stop", model.events[0].Action)

		msgFail := actionResultMsg{
			success:   false,
			action:    "restart",
			container: "container-1",
			err:       fmt.Errorf("restart failed"),
		}
		newM2, cmd2 := model.Update(msgFail)
		assert.NotNil(t, cmd2) // tea.Batch
		model2 := newM2.(Model)
		assert.Len(t, model2.events, 2)
		assert.Equal(t, "restart FAILED", model2.events[0].Action)
	})

	t.Run("stressResultMsg", func(t *testing.T) {
		freshM := Model{events: []EventLogEntry{}}
		msg := stressResultMsg{
			success: true,
			mode:    "cpu",
		}
		newM, cmd := freshM.Update(msg)
		assert.NotNil(t, cmd) // tea.Batch
		model := newM.(Model)
		assert.Len(t, model.events, 1)
		assert.Equal(t, "stress-cpu (30s)", model.events[0].Name)
		assert.Equal(t, "stress", model.events[0].Action)

		msgFail := stressResultMsg{
			success: false,
			mode:    "mem",
			err:     fmt.Errorf("stress test failed"),
		}
		newM2, cmd2 := model.Update(msgFail)
		assert.NotNil(t, cmd2) // tea.Batch
		model2 := newM2.(Model)
		assert.Len(t, model2.events, 2)
		assert.Contains(t, model2.events[0].Name, "stress test failed")
	})

	t.Run("diskUsageMsg", func(t *testing.T) {
		du := docker.SystemDiskUsage{ImagesReclaimable: 100, VolumesReclaimable: 200}
		msg := diskUsageMsg{usage: du}
		newM, cmd := m.Update(msg)
		assert.Nil(t, cmd)
		model := newM.(Model)
		assert.Equal(t, int64(100), model.diskUsage.ImagesReclaimable)
		assert.Equal(t, int64(200), model.diskUsage.VolumesReclaimable)
	})

	t.Run("pruneResultMsg", func(t *testing.T) {
		freshM := Model{events: []EventLogEntry{}}
		msg := pruneResultMsg{
			reclaimed: 1024,
			target:    "images",
			err:       nil,
		}
		newM, cmd := freshM.Update(msg)
		assert.NotNil(t, cmd) // m.fetchDiskUsage
		model := newM.(Model)
		assert.False(t, model.pruning)
		assert.Contains(t, model.pruneFeedback, "1.0KB")
		assert.Len(t, model.events, 1)
		assert.Equal(t, "prune images", model.events[0].Action)
	})

	t.Run("hostStatsMsg", func(t *testing.T) {
		msg := hostStatsMsg{cpu: 55.5, mem: 66.6}
		newM, cmd := m.Update(msg)
		assert.Nil(t, cmd)
		model := newM.(Model)
		assert.Equal(t, 55.5, model.hostCPU)
		assert.Equal(t, 66.6, model.hostMem)
	})

	t.Run("tickMsg", func(t *testing.T) {
		msg := tickMsg(time.Now())
		newM, cmd := m.Update(msg)
		assert.NotNil(t, cmd)
		_ = newM.(Model)
	})

	t.Run("unknown message", func(t *testing.T) {
		msg := "unknown message"
		newM, cmd := m.Update(msg)
		assert.Nil(t, cmd)
		assert.Equal(t, m, newM.(Model))
	})
}
