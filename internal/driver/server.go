package driver

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
)

func (d *Driver) Run() error {
	if err := d.validateConfig(); err != nil {
		return err
	}
	network, address, err := parseEndpoint(d.config.Endpoint)
	if err != nil {
		return err
	}
	if network == "unix" {
		if err := os.MkdirAll(filepath.Dir(address), 0o750); err != nil {
			return fmt.Errorf("create socket directory: %w", err)
		}
		if err := os.Remove(address); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale socket: %w", err)
		}
	}

	listener, err := net.Listen(network, address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", d.config.Endpoint, err)
	}
	defer listener.Close()

	if d.config.OrphanReclaim {
		if d.pvs == nil {
			d.pvs, err = inClusterPVLister(d.config.NodeID)
			if err != nil {
				d.log.Warn("orphan reconciliation disabled", "error", err)
			}
		}
		if d.pvs != nil {
			go d.reconcileLoop()
		}
	}

	server := grpc.NewServer()
	csi.RegisterIdentityServer(server, d)
	csi.RegisterControllerServer(server, d)
	csi.RegisterNodeServer(server, d)
	d.log.Info("serving CSI", "endpoint", d.config.Endpoint, "node", d.config.NodeID)
	return server.Serve(listener)
}

func parseEndpoint(endpoint string) (string, string, error) {
	switch {
	case strings.HasPrefix(endpoint, "unix://"):
		return "unix", strings.TrimPrefix(endpoint, "unix://"), nil
	case strings.HasPrefix(endpoint, "unix:"):
		return "unix", strings.TrimPrefix(endpoint, "unix:"), nil
	default:
		return "", "", fmt.Errorf("unsupported endpoint %q", endpoint)
	}
}
