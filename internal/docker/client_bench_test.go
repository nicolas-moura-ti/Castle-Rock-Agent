package docker_test

import (
	"context"
	"testing"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
)

func BenchmarkListRunningContainersDetailed(b *testing.B) {
	client, err := docker.NewClient()
	if err != nil {
		b.Fatalf("failed to create docker client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.ListRunningContainersDetailed(ctx, true)
		if err != nil {
			b.Fatalf("failed to list containers detailed: %v", err)
		}
	}
}
