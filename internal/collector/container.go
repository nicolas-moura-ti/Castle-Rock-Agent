// Package collector define as interfaces e implementações para coleta
// de métricas de containers Docker.
//
// Este package é o coração do sistema de observabilidade. Ele define
// a interface Collector que abstrai a lógica de coleta, permitindo
// diferentes implementações (CPU, memória, rede, disco).
//
// ARQUITETURA:
//   - Collector é uma interface (contrato)
//   - Cada tipo de métrica terá sua própria implementação
//   - O orquestrador (main ou um scheduler) chama Collect() periodicamente
//   - Segue o princípio Open/Closed: aberto para extensão, fechado para modificação
package collector

import (
	"context"

	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// Collector define o contrato para coletores de métricas.
//
// INTERFACES EM GO:
//   - Interfaces são definidas pelo consumidor, não pelo implementador
//   - Uma interface pequena é melhor que uma grande (Interface Segregation)
//   - A convenção Go é: "Accept interfaces, return structs"
//   - Não crie interfaces prematuramente — extraia-as quando houver
//     necessidade real de polimorfismo
//
// Por que definir esta interface agora?
//   - Estabelece o contrato que futuras implementações devem seguir
//   - Permite uso de mocks em testes unitários
//   - Documenta a intenção arquitetural do sistema
type Collector interface {
	// Collect executa a coleta de métricas para todos os containers.
	//
	// Aceita um context para suportar cancelamento e timeouts.
	// Retorna um slice de ContainerMetrics ou um erro.
	Collect(ctx context.Context) ([]models.ContainerMetrics, error)

	// Name retorna o nome do coletor (ex: "cpu", "memory", "network").
	// Usado para logging e identificação.
	Name() string
}

// ContainerCollector é um scaffold para o coletor principal de containers.
// Futuramente, esta struct será expandida com as dependências necessárias
// (Docker client, configuração, etc.).
type ContainerCollector struct {
	// TODO: Adicionar dependências quando implementar a coleta real.
	// Exemplo:
	//   dockerClient *docker.Client
	//   interval     time.Duration
}

// NewContainerCollector cria uma nova instância do ContainerCollector.
//
// Segue o padrão constructor New<Tipo> do Go.
func NewContainerCollector() *ContainerCollector {
	return &ContainerCollector{}
}

// Collect implementa a interface Collector.
//
// TODO: Implementar coleta real de métricas via Docker Stats API.
// A Docker Stats API fornece métricas em tempo real de CPU, memória,
// I/O de rede e disco para cada container.
func (c *ContainerCollector) Collect(ctx context.Context) ([]models.ContainerMetrics, error) {
	// Placeholder — será implementado na próxima iteração.
	return nil, nil
}

// Name retorna o nome deste coletor.
func (c *ContainerCollector) Name() string {
	return "container"
}
