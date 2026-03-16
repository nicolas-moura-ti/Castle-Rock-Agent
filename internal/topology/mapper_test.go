package topology

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
)

func TestMapper_BuildMap(t *testing.T) {
	// Setup a mock HTTP server for the Docker daemon API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock Docker API negotiation and health check
		if r.URL.Path == "/_ping" || r.URL.Path == "/v1.43/version" {
			w.Header().Set("Api-Version", "1.43")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ApiVersion":"1.43"}`))
			return
		}

		// Handle /v1.43/networks list request
		if r.URL.Path == "/v1.43/networks" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Return a list of 4 networks:
			// 1. my_network (valid, with containers)
			// 2. host (built-in, should be skipped)
			// 3. none (built-in, should be skipped)
			// 4. empty_network (no containers, should be skipped)
			w.Write([]byte(`[
				{"Id": "net1_id1234567890", "Name": "my_network"},
				{"Id": "net2_id1234567890", "Name": "host"},
				{"Id": "net3_id1234567890", "Name": "none"},
				{"Id": "net4_id1234567890", "Name": "empty_network"}
			]`))
			return
		}

		// Handle specific network inspect requests
		if r.URL.Path == "/v1.43/networks/net1_id1234567890" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Network with one container connected
			w.Write([]byte(`{
				"Id": "net1_id1234567890",
				"Name": "my_network",
				"Driver": "bridge",
				"Containers": {
					"container1_id1234567890": {
						"Name": "my_container",
						"EndpointID": "endpoint1_id",
						"MacAddress": "02:42:ac:11:00:02",
						"IPv4Address": "172.17.0.2/16",
						"IPv6Address": ""
					}
				}
			}`))
			return
		}

		if r.URL.Path == "/v1.43/networks/net2_id1234567890" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"Id": "net2_id1234567890", "Name": "host", "Driver": "host", "Containers": {"container2_id1234567890": {"Name": "host_container", "IPv4Address": "127.0.0.1/8"}}}`))
			return
		}

		if r.URL.Path == "/v1.43/networks/net3_id1234567890" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"Id": "net3_id1234567890", "Name": "none", "Driver": "null", "Containers": {}}`))
			return
		}

		if r.URL.Path == "/v1.43/networks/net4_id1234567890" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"Id": "net4_id1234567890", "Name": "empty_network", "Driver": "bridge", "Containers": {}}`))
			return
		}

		// Default fallback
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Redirect Docker client to the mock server
	t.Setenv("DOCKER_HOST", server.URL)
	t.Setenv("DOCKER_API_VERSION", "1.43")

	// Create a new Docker client wrapper
	client, err := docker.NewClient()
	if err != nil {
		t.Fatalf("Failed to create docker client: %v", err)
	}
	defer client.Close()

	// Initialize Mapper
	mapper := NewMapper(client)

	// Call BuildMap
	edges, err := mapper.BuildMap(context.Background())
	if err != nil {
		t.Fatalf("BuildMap returned error: %v", err)
	}

	// Validate the result
	// Expected: only 1 network should be returned ("my_network")
	// "host" and "none" skipped by explicit rules
	// "empty_network" skipped because len(nodes) == 0
	if len(edges) != 1 {
		t.Fatalf("Expected 1 network edge, got %d", len(edges))
	}

	edge := edges[0]
	if edge.NetworkName != "my_network" {
		t.Errorf("Expected NetworkName 'my_network', got '%s'", edge.NetworkName)
	}
	if edge.NetworkID != "net1_id12345" { // truncated to 12 chars: net1_id12345
		t.Errorf("Expected NetworkID 'net1_id12345', got '%s'", edge.NetworkID)
	}
	if edge.Driver != "bridge" {
		t.Errorf("Expected Driver 'bridge', got '%s'", edge.Driver)
	}

	if len(edge.Nodes) != 1 {
		t.Fatalf("Expected 1 node in the network, got %d", len(edge.Nodes))
	}

	node := edge.Nodes[0]
	if node.ContainerName != "my_container" {
		t.Errorf("Expected ContainerName 'my_container', got '%s'", node.ContainerName)
	}
	if node.ContainerID != "container1_i" { // truncated to 12 chars
		t.Errorf("Expected ContainerID 'container1_i', got '%s'", node.ContainerID)
	}
	if node.IPv4Address != "172.17.0.2" { // stripped of /16
		t.Errorf("Expected IPv4Address '172.17.0.2', got '%s'", node.IPv4Address)
	}
}

func TestMapper_BuildMap_Error(t *testing.T) {
	// Setup a mock HTTP server that returns an error for ListNetworks
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" || r.URL.Path == "/v1.43/version" {
			w.Header().Set("Api-Version", "1.43")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ApiVersion":"1.43"}`))
			return
		}

		if r.URL.Path == "/v1.43/networks" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"message": "internal server error"}`))
			return
		}
	}))
	defer server.Close()

	t.Setenv("DOCKER_HOST", server.URL)
	t.Setenv("DOCKER_API_VERSION", "1.43")

	client, err := docker.NewClient()
	if err != nil {
		t.Fatalf("Failed to create docker client: %v", err)
	}
	defer client.Close()

	mapper := NewMapper(client)

	edges, err := mapper.BuildMap(context.Background())
	if err == nil {
		t.Fatal("Expected error from BuildMap, but got nil")
	}
	if edges != nil {
		t.Fatalf("Expected nil edges on error, got %v", edges)
	}
}
