// Package models define os DTOs (Data Transfer Objects) e structs
// públicas do Castle Rock Agent.
//
// Este package fica em pkg/ (não internal/) porque pode ser importado
// por outros projetos que queiram consumir os dados do agente.
//
// CONVENÇÃO GO:
//   - pkg/ = código público, reutilizável por projetos externos
//   - internal/ = código privado, restrito ao módulo atual
//   - A distinção é enforçada pelo compilador Go (não apenas convenção)
package models

// ContainerInfo representa as informações básicas de um container Docker.
//
// Esta struct é o DTO principal para a listagem de containers.
// Contém apenas dados de identificação e estado — métricas de performance
// ficam em ContainerMetrics.
//
// BOAS PRÁTICAS EM STRUCTS GO:
//   - Campos exportados (maiúsculos) para serialização JSON
//   - Tags JSON para controlar os nomes no output serializado
//   - Documentação em cada campo para auto-documentação da API
type ContainerInfo struct {
	// ID é o identificador único do container (short ID, 12 caracteres).
	ID string `json:"id"`

	// Name é o nome atribuído ao container (sem o prefixo "/").
	Name string `json:"name"`

	// Image é o nome da imagem Docker usada pelo container.
	Image string `json:"image"`

	// Status é a descrição legível do estado (ex: "Up 2 hours").
	Status string `json:"status"`

	// State é o estado técnico do container (running, exited, paused, etc.).
	State string `json:"state"`

	// Ports são as portas expostas formatadas (ex: "0.0.0.0:8080->80/tcp").
	Ports string `json:"ports,omitempty"`
}

// ContainerMetrics representa as métricas de performance de um container.
//
// Este DTO será populado pelo collector usando a Docker Stats API.
// A separação entre ContainerInfo e ContainerMetrics segue o princípio
// de responsabilidade única: info é estático, metrics é dinâmico.
type ContainerMetrics struct {
	// ContainerID identifica o container ao qual estas métricas pertencem.
	ContainerID string `json:"container_id"`

	// ContainerName é o nome do container (para labels de métricas).
	ContainerName string `json:"container_name"`

	// Image é a imagem Docker do container.
	Image string `json:"image"`

	// CPUPercent é o percentual de uso de CPU do container.
	CPUPercent float64 `json:"cpu_percent"`

	// MemoryUsage é o uso de memória em bytes.
	MemoryUsage uint64 `json:"memory_usage"`

	// MemoryLimit é o limite de memória configurado em bytes.
	MemoryLimit uint64 `json:"memory_limit"`

	// MemoryPercent é o percentual de uso de memória.
	MemoryPercent float64 `json:"memory_percent"`

	// NetworkRx é o total de bytes recebidos pela rede.
	NetworkRx uint64 `json:"network_rx"`

	// NetworkTx é o total de bytes transmitidos pela rede.
	NetworkTx uint64 `json:"network_tx"`

	// BlockRead é o total de bytes lidos do disco.
	BlockRead uint64 `json:"block_read"`

	// BlockWrite é o total de bytes escritos no disco.
	BlockWrite uint64 `json:"block_write"`
}
