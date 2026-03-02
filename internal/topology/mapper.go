package topology

import (
	"context"
	"strings"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
)

// NetworkNode representa um nó na topologia
type NetworkNode struct {
	ContainerName string `json:"container_name"`
	ContainerID   string `json:"container_id"`
	IPv4Address   string `json:"ipv4_address"`
}

// NetworkEdge representa uma conexão ou rede
type NetworkEdge struct {
	NetworkName string        `json:"network_name"`
	NetworkID   string        `json:"network_id"`
	Driver      string        `json:"driver"`
	Nodes       []NetworkNode `json:"nodes"`
}

// Mapper é o extrator da topologia visual de containers
type Mapper struct {
	dockerClient *docker.Client
}

// NewMapper inicia o serviço de topologia
func NewMapper(client *docker.Client) *Mapper {
	return &Mapper{dockerClient: client}
}

// BuildMap gera o agrupamento de containers por de rede
func (m *Mapper) BuildMap(ctx context.Context) ([]NetworkEdge, error) {
	nets, err := m.dockerClient.ListNetworks(ctx)
	if err != nil {
		return nil, err
	}

	var edges []NetworkEdge

	for _, n := range nets {
		// Ignora networks built-in vazias de utilidade
		if n.Name == "host" || n.Name == "none" {
			continue
		}

		var nodes []NetworkNode
		for id, endpoint := range n.Containers {
			// EndpointName geralmente tem o nome limpo s/ barra
			name := endpoint.Name
			nodes = append(nodes, NetworkNode{
				ContainerName: name,
				ContainerID:   id[:12],
				IPv4Address:   strings.Split(endpoint.IPv4Address, "/")[0],
			})
		}

		// Só retorna redes com containers ativamente plugados (ignora redes vazias)
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
