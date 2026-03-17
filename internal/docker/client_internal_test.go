package docker

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
)

func TestCalculateCPUPercent(t *testing.T) {
	tests := []struct {
		name     string
		stats    *types.StatsJSON
		expected float64
	}{
		{
			name: "normal calculation with OnlineCPUs",
			stats: &types.StatsJSON{
				Stats: types.Stats{
					CPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage: 200,
						},
						SystemUsage: 400,
						OnlineCPUs:  2,
					},
					PreCPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage: 100,
						},
						SystemUsage: 200,
					},
				},
			},
			// cpuDelta = 100
			// systemDelta = 200
			// numCPUs = 2
			// (100 / 200) * 2 * 100.0 = 100.0
			expected: 100.0,
		},
		{
			name: "fallback to PercpuUsage length when OnlineCPUs is 0",
			stats: &types.StatsJSON{
				Stats: types.Stats{
					CPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage:  150,
							PercpuUsage: []uint64{50, 50, 50}, // length 3
						},
						SystemUsage: 300,
						OnlineCPUs:  0,
					},
					PreCPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage: 50,
						},
						SystemUsage: 100,
					},
				},
			},
			// cpuDelta = 100
			// systemDelta = 200
			// numCPUs = 3
			// (100 / 200) * 3 * 100.0 = 150.0
			expected: 150.0,
		},
		{
			name: "fallback to 1 when OnlineCPUs and PercpuUsage are 0/empty",
			stats: &types.StatsJSON{
				Stats: types.Stats{
					CPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage:  150,
							PercpuUsage: []uint64{},
						},
						SystemUsage: 300,
						OnlineCPUs:  0,
					},
					PreCPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage: 50,
						},
						SystemUsage: 100,
					},
				},
			},
			// cpuDelta = 100
			// systemDelta = 200
			// numCPUs = 1
			// (100 / 200) * 1 * 100.0 = 50.0
			expected: 50.0,
		},
		{
			name: "zero system delta",
			stats: &types.StatsJSON{
				Stats: types.Stats{
					CPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage: 200,
						},
						SystemUsage: 200,
						OnlineCPUs:  2,
					},
					PreCPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage: 100,
						},
						SystemUsage: 200, // systemDelta = 0
					},
				},
			},
			expected: 0.0,
		},
		{
			name: "zero cpu delta",
			stats: &types.StatsJSON{
				Stats: types.Stats{
					CPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage: 100,
						},
						SystemUsage: 400,
						OnlineCPUs:  2,
					},
					PreCPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage: 100, // cpuDelta = 0
						},
						SystemUsage: 200,
					},
				},
			},
			expected: 0.0,
		},
		{
			name: "negative system delta",
			stats: &types.StatsJSON{
				Stats: types.Stats{
					CPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage: 200,
						},
						SystemUsage: 100,
						OnlineCPUs:  2,
					},
					PreCPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage: 100,
						},
						SystemUsage: 200, // systemDelta = -100
					},
				},
			},
			expected: 0.0,
		},
		{
			name: "negative cpu delta",
			stats: &types.StatsJSON{
				Stats: types.Stats{
					CPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage: 100,
						},
						SystemUsage: 400,
						OnlineCPUs:  2,
					},
					PreCPUStats: container.CPUStats{
						CPUUsage: container.CPUUsage{
							TotalUsage: 200, // cpuDelta = -100
						},
						SystemUsage: 200,
					},
				},
			},
			expected: 0.0,
		},
		{
			name:     "nil stats",
			stats:    nil,
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateCPUPercent(tt.stats)
			assert.Equal(t, tt.expected, result)
		})
	}
}
