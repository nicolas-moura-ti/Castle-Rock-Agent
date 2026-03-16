package cluster

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"crypto/subtle"
	"sync"
	"time"

	"github.com/nicolas-moura-ti/castle-rock-agent/pkg/models"
)

// HostData stores the latest metrics/containers snapshot received from a Worker.
type HostData struct {
	LastUpdate time.Time
	Payload    models.PushPayload
}

// Receiver is the HTTP server integrated in the Leader to receive data from Workers.
type Receiver struct {
	mu           sync.RWMutex
	hosts        map[string]HostData
	log          *slog.Logger
	sharedSecret string // Usado para descriptografia AES
	token        string // Usado para o header Authorization: Bearer
}

// NewReceiver creates a new Receiver instance.
// Resolvido: Segredo AES e Token Bearer agora são chaves separadas.
func NewReceiver(log *slog.Logger, secret string, token string) *Receiver {
	return &Receiver{
		hosts:        make(map[string]HostData),
		log:          log,
		sharedSecret: secret,
		token:        token,
	}
}

// ServeHTTP implements the http.Handler interface to receive POSTs with the Payload.
func (r *Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Validação do Token (Authorization Header)
	if r.token != "" {
		authHeader := req.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			r.log.Warn("receiver: missing or invalid authorization header")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		providedToken := strings.TrimPrefix(authHeader, "Bearer ")
		// Uso de subtle.ConstantTimeCompare previne ataques de timing
		if subtle.ConstantTimeCompare([]byte(providedToken), []byte(r.token)) != 1 {
			r.log.Warn("receiver: invalid token provided")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// 1.5 Correção de Vulnerabilidade: Prevendo ataques DoS por exaustão de memória
	// Limita o tamanho do body a 5MB, blindando o Agente contra payloads corrompidos.
	req.Body = http.MaxBytesReader(w, req.Body, 5*1024*1024)
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		r.log.Warn("receiver: failed to read body", slog.String("error", err.Error()))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// 2. Lógica de Descriptografia (da branch fix/encrypt)
	isEncrypted := req.Header.Get("X-CastleRock-Encrypted") == "true"

	if r.sharedSecret != "" {
		if !isEncrypted {
			r.log.Warn("receiver: rejected cleartext payload (SharedSecret is enforced)")
			http.Error(w, "Forbidden: Encryption Required", http.StatusForbidden)
			return
		}

		// Assume-se que a função Decrypt existe no pacote cluster
		decryptedBody, err := Decrypt(body, r.sharedSecret)
		if err != nil {
			r.log.Warn("receiver: failed to decrypt payload", slog.String("error", err.Error()))
			http.Error(w, "Forbidden: Decryption Failed", http.StatusForbidden)
			return
		}
		body = decryptedBody
	} else {
		if isEncrypted {
			r.log.Warn("receiver: rejected encrypted payload (no SharedSecret configured locally)")
			http.Error(w, "Forbidden: Decryption Key Missing", http.StatusForbidden)
			return
		}
	}

	// 3. Processamento do JSON
	var payload models.PushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		r.log.Warn("receiver: failed to decode JSON", slog.String("error", err.Error()))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if payload.HostID == "" {
		r.log.Warn("receiver: payload rejected, missing HostID")
		http.Error(w, "Missing HostID", http.StatusBadRequest)
		return
	}

	r.mu.Lock()
	r.hosts[payload.HostID] = HostData{
		LastUpdate: time.Now(),
		Payload:    payload,
	}
	r.mu.Unlock()

	w.WriteHeader(http.StatusAccepted)
}

// GetAllMetrics returns a list containing all recent metrics from all Workers.
func (r *Receiver) GetAllMetrics() []models.ContainerMetrics {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var all []models.ContainerMetrics
	now := time.Now()

	for hostID, data := range r.hosts {
		if now.Sub(data.LastUpdate) > 30*time.Second {
			r.log.Debug("receiver: inactive host ignored", slog.String("host", hostID))
			continue
		}
		all = append(all, data.Payload.Metrics...)
	}

	return all
}

// GetAllContainers returns static container info from all recent Workers.
func (r *Receiver) GetAllContainers() []models.ContainerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var all []models.ContainerInfo
	now := time.Now()

	for _, data := range r.hosts {
		if now.Sub(data.LastUpdate) > 30*time.Second {
			continue
		}
		all = append(all, data.Payload.Containers...)
	}

	return all
}