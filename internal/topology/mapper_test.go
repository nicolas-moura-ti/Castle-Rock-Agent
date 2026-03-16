package topology

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nicolas-moura-ti/castle-rock-agent/internal/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMockServer(t *testing.T, listNetworksHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock Docker API negotiation and health check
		if r.URL.Path == "/_ping" || r.URL.Path == "/v1.43/version" {
			w.Header().Set("Api-Version", "1.43")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ApiVersion":"1.43"}`))
			return
		}

		if r.URL.Path == "/v1.43/networks" {
			listNetworksHandler(w, r)
			return
		}

		// Handle specific network inspect requests
		if r.URL.Path == "/v1.43/networks/net1_id1234567890" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
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
}

func TestMapper_BuildMap(t *testing.T) {
	server := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{"Id": "net1_id1234567890", "Name": "my_network"},
			{"Id": "net2_id1234567890", "Name": "host"},
			{"Id": "net3_id1234567890", "Name": "none"},
			{"Id": "net4_id1234567890", "Name": "empty_network"}
		]`))
	})
	defer server.Close()

	t.Setenv("DOCKER_HOST", server.URL)
	t.Setenv("DOCKER_API_VERSION", "1.43")

	client, err := docker.NewClient()
	require.NoError(t, err)
	defer client.Close()

	mapper := NewMapper(client)
	edges, err := mapper.BuildMap(context.Background())
	require.NoError(t, err)

	require.Len(t, edges, 1, "Expected 1 network edge")
	edge := edges[0]
	assert.Equal(t, "my_network", edge.NetworkName)
	assert.Equal(t, "net1_id12345", edge.NetworkID)
	assert.Equal(t, "bridge", edge.Driver)

	require.Len(t, edge.Nodes, 1, "Expected 1 node in the network")
	node := edge.Nodes[0]
	assert.Equal(t, "my_container", node.ContainerName)
	assert.Equal(t, "container1_i", node.ContainerID)
	assert.Equal(t, "172.17.0.2", node.IPv4Address)
}

func TestMapper_BuildMap_Error(t *testing.T) {
	server := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "internal server error"}`))
	})
	defer server.Close()

	t.Setenv("DOCKER_HOST", server.URL)
	t.Setenv("DOCKER_API_VERSION", "1.43")

	client, err := docker.NewClient()
	require.NoError(t, err)
	defer client.Close()

	mapper := NewMapper(client)
	edges, err := mapper.BuildMap(context.Background())
	assert.Error(t, err)
	assert.Nil(t, edges)
}
