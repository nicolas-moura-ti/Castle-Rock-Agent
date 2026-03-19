package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/hashicorp/mdns"
)

const (
	ServiceType = "_castlerock._tcp"
	ServiceName = "Castle Rock Leader"
)

// Advertiser handles mDNS service registration for the Leader.
type Advertiser struct {
	server *mdns.Server
	log    *slog.Logger
}

// NewAdvertiser starts advertising the Leader's metrics port on mDNS.
func NewAdvertiser(port int, log *slog.Logger) (*Advertiser, error) {
	ips, _ := net.LookupIP("localhost")
	info := []string{"Castle Rock Observability Leader"}
	service, err := mdns.NewMDNSService("castlerock-leader", ServiceType, "", "", port, ips, info)
	if err != nil {
		return nil, err
	}

	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		return nil, err
	}

	log.Info("mDNS: Advertising Castle Rock Leader", slog.Int("port", port))
	return &Advertiser{server: server, log: log}, nil
}

func (a *Advertiser) Close() error {
	return a.server.Shutdown()
}

// DiscoverLeader searches for a Castle Rock Leader on the local network.
func DiscoverLeader(ctx context.Context, log *slog.Logger) (string, error) {
	log.Info("mDNS: Searching for Castle Rock Leader...")

	entriesCh := make(chan *mdns.ServiceEntry, 4)

	// Start the lookup
	go func() {
		params := &mdns.QueryParam{
			Service: ServiceType,
			Domain:  "local",
			Timeout: 5 * time.Second,
			Entries: entriesCh,
		}
		if err := mdns.Query(params); err != nil {
			log.Warn("mDNS: Query failed", slog.String("error", err.Error()))
		}
		close(entriesCh)
	}()

	select {
	case entry := <-entriesCh:
		if entry != nil {
			url := fmt.Sprintf("http://%s:%d/api/v1/push", entry.AddrV4.String(), entry.Port)
			log.Info("mDNS: Found Leader", slog.String("url", url))
			return url, nil
		}
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(6 * time.Second):
		return "", fmt.Errorf("mDNS: timeout searching for leader")
	}

	return "", fmt.Errorf("mDNS: leader not found")
}
