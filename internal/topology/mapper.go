package topology

import (
	"context"
	"strings"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
)

// NetworkNode represents a node in the topology.
type NetworkNode struct {
	ContainerName string `json:"container_name"`
	ContainerID   string `json:"container_id"`
	IPv4Address   string `json:"ipv4_address"`
}

// NetworkEdge represents a connection or network.
type NetworkEdge struct {
	NetworkName string        `json:"network_name"`
	NetworkID   string        `json:"network_id"`
	Driver      string        `json:"driver"`
	Nodes       []NetworkNode `json:"nodes"`
}

// Mapper is the visual container topology extractor.
type Mapper struct {
	dockerClient docker.ContainerEngine
}

// NewMapper initializes the topology service.
func NewMapper(client docker.ContainerEngine) *Mapper {
	return &Mapper{dockerClient: client}
}

// BuildMap generates the container grouping by network.
func (m *Mapper) BuildMap(ctx context.Context) ([]NetworkEdge, error) {
	nets, err := m.dockerClient.ListNetworks(ctx)
	if err != nil {
		return nil, err
	}

	edges := make([]NetworkEdge, 0, len(nets))

	for _, n := range nets {
		// Skip empty built-in networks
		if n.Name == "host" || n.Name == "none" {
			continue
		}

		nodes := make([]NetworkNode, 0, len(n.Containers))
		for id, endpoint := range n.Containers {
			// EndpointName usually has the clean name without slash
			name := endpoint.Name

			ip := endpoint.IPv4Address
			if idx := strings.IndexByte(ip, '/'); idx != -1 {
				ip = ip[:idx]
			}

			idStr := id
			if len(idStr) > 12 {
				idStr = idStr[:12]
			}

			nodes = append(nodes, NetworkNode{
				ContainerName: name,
				ContainerID:   idStr,
				IPv4Address:   ip,
			})
		}

		// Only return networks with actively connected containers (skip empty ones)
		if len(nodes) > 0 {
			edges = append(edges, NetworkEdge{
				NetworkName: n.Name,
				NetworkID:   n.ID[:12],
				Driver:      n.Driver,
				Nodes:       nodes,
			})
		}
	}

	return edges, nil
}
