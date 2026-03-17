package cluster

import (
	"compress/gzip"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// ─────────────────────────────────────────────────────────────────────────────
// DATA STORAGE (SRP / ISP)
// ─────────────────────────────────────────────────────────────────────────────

// HostData stores the latest metrics/containers snapshot received from a Worker.
type HostData struct {
	LastUpdate time.Time
	Payload    models.PushPayload
}

// ClusterStore defines the interface for managing data received from Workers.
// This abstraction allows swapping the in-memory map for Redis or SQLite
// without touching the HTTP Receiver (DIP).
type ClusterStore interface {
	UpdateHost(payload models.PushPayload)
	GetAllMetrics() []models.ContainerMetrics
	GetAllContainers() []models.ContainerInfo
}

// MemoryStore is a sharded in-memory implementation of ClusterStore.
// Sharding reduces lock contention by splitting the map into multiple buckets,
// each with its own Mutex. This allows concurrent updates to different shards.
type MemoryStore struct {
	shards    [16]*shard
	shardMask uint32
	log       *slog.Logger
}

type shard struct {
	mu    sync.RWMutex
	hosts map[string]HostData
}

func NewMemoryStore(log *slog.Logger) *MemoryStore {
	s := &MemoryStore{
		shardMask: 15, // 16 shards - 1
		log:       log,
	}
	for i := 0; i < 16; i++ {
		s.shards[i] = &shard{
			hosts: make(map[string]HostData),
		}
	}
	return s
}

// hashHostID computes a simple hash for shard distribution.
func (s *MemoryStore) getShard(hostID string) *shard {
	var h uint32 = 2166136261
	for i := 0; i < len(hostID); i++ {
		h = (h ^ uint32(hostID[i])) * 16777619
	}
	return s.shards[h&s.shardMask]
}

func (s *MemoryStore) UpdateHost(payload models.PushPayload) {
	shard := s.getShard(payload.HostID)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	shard.hosts[payload.HostID] = HostData{
		LastUpdate: time.Now(),
		Payload:    payload,
	}
}

func (s *MemoryStore) GetAllMetrics() []models.ContainerMetrics {
	var all []models.ContainerMetrics
	now := time.Now()

	for _, shard := range s.shards {
		shard.mu.RLock()
		for hostID, data := range shard.hosts {
			if now.Sub(data.LastUpdate) > 30*time.Second {
				s.log.Debug("store: inactive host ignored", slog.String("host", hostID))
				continue
			}
			all = append(all, data.Payload.Metrics...)
		}
		shard.mu.RUnlock()
	}
	return all
}

func (s *MemoryStore) GetAllContainers() []models.ContainerInfo {
	var all []models.ContainerInfo
	now := time.Now()

	for _, shard := range s.shards {
		shard.mu.RLock()
		for _, data := range shard.hosts {
			if now.Sub(data.LastUpdate) > 30*time.Second {
				continue
			}
			all = append(all, data.Payload.Containers...)
		}
		shard.mu.RUnlock()
	}
	return all
}

// ─────────────────────────────────────────────────────────────────────────────
// TRANSPORT LAYER (SRP)
// ─────────────────────────────────────────────────────────────────────────────

// Receiver is the HTTP server integrated in the Leader to receive data from Workers.
// It is only responsible for Authentication, Decryption and Routing.
type Receiver struct {
	store        ClusterStore
	log          *slog.Logger
	sharedSecret string // Used for AES decryption
	token        string // Used for Bearer token validation
}

// NewReceiver creates a new Receiver instance injecting a store.
func NewReceiver(log *slog.Logger, store ClusterStore, secret string, token string) *Receiver {
	return &Receiver{
		store:        store,
		log:          log,
		sharedSecret: secret,
		token:        token,
	}
}

// ServeHTTP implements the http.Handler interface.
func (r *Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Token Validation
	if r.token != "" {
		authHeader := req.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			r.log.Warn("receiver: missing or invalid auth header")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		providedToken := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(providedToken), []byte(r.token)) != 1 {
			r.log.Warn("receiver: invalid token")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// 2. Body Read & Size Limit (Security Protection)
	req.Body = http.MaxBytesReader(w, req.Body, 5*1024*1024)
	defer req.Body.Close()

	var reader io.ReadCloser = req.Body
	if req.Header.Get("Content-Encoding") == "gzip" {
		var err error
		reader, err = gzip.NewReader(req.Body)
		if err != nil {
			r.log.Warn("receiver: gzip decompress error", slog.String("error", err.Error()))
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		defer reader.Close()
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		r.log.Warn("receiver: read failure", slog.String("error", err.Error()))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// 3. Decryption Logic
	isEncrypted := req.Header.Get("X-CastleRock-Encrypted") == "true"
	if r.sharedSecret != "" {
		if !isEncrypted {
			r.log.Warn("receiver: rejected cleartext payload (encryption required)")
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		decryptedBody, err := Decrypt(body, r.sharedSecret)
		if err != nil {
			r.log.Warn("receiver: decryption failure", slog.String("error", err.Error()))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		body = decryptedBody
	}

	// 4. JSON Processing
	var payload models.PushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		r.log.Warn("receiver: json decode error", slog.String("error", err.Error()))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if payload.HostID == "" {
		http.Error(w, "Missing HostID", http.StatusBadRequest)
		return
	}

	// 5. Delegate to Store (SRP)
	r.store.UpdateHost(payload)

	w.WriteHeader(http.StatusAccepted)
}

// Proxied methods to maintain compatibility if needed, 
// though direct access to store is preferred.
func (r *Receiver) GetAllMetrics() []models.ContainerMetrics {
	return r.store.GetAllMetrics()
}

func (r *Receiver) GetAllContainers() []models.ContainerInfo {
	return r.store.GetAllContainers()
}
