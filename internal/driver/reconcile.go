package driver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type pvLister interface {
	List(context.Context) (map[string]struct{}, error)
}

type apiPVLister struct {
	url       string
	tokenPath string
	client    *http.Client
	nodeID    string
}

const serviceAccountPath = "/var/run/secrets/kubernetes.io/serviceaccount"

func inClusterPVLister(nodeID string) (pvLister, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, fmt.Errorf("Kubernetes service environment is not present")
	}
	tokenPath := filepath.Join(serviceAccountPath, "token")
	if _, err := os.Stat(tokenPath); err != nil {
		return nil, fmt.Errorf("stat service account token: %w", err)
	}
	ca, err := os.ReadFile(filepath.Join(serviceAccountPath, "ca.crt"))
	if err != nil {
		return nil, fmt.Errorf("read service account CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("parse service account CA")
	}
	return &apiPVLister{
		url:       "https://" + net.JoinHostPort(host, port) + "/api/v1/persistentvolumes",
		tokenPath: tokenPath,
		client:    &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}},
		nodeID:    nodeID,
	}, nil
}

func (l *apiPVLister) List(ctx context.Context) (map[string]struct{}, error) {
	token, err := os.ReadFile(l.tokenPath)
	if err != nil {
		return nil, fmt.Errorf("read service account token: %w", err)
	}
	if len(strings.TrimSpace(string(token))) == 0 {
		return nil, fmt.Errorf("service account token is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	response, err := l.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list PVs: Kubernetes API returned %s", response.Status)
	}
	var list persistentVolumeList
	if err := json.NewDecoder(response.Body).Decode(&list); err != nil {
		return nil, err
	}
	volumes := make(map[string]struct{})
	for _, item := range list.Items {
		if item.Spec.CSI == nil || item.Spec.CSI.Driver != driverName || !pvBelongsToNode(item, l.nodeID) {
			continue
		}
		volumes[item.Spec.CSI.VolumeHandle] = struct{}{}
	}
	return volumes, nil
}

type persistentVolumeList struct {
	Items []persistentVolume `json:"items"`
}

type persistentVolume struct {
	Spec struct {
		CSI *struct {
			Driver       string `json:"driver"`
			VolumeHandle string `json:"volumeHandle"`
		} `json:"csi"`
		NodeAffinity *struct {
			Required *struct {
				Terms []struct {
					Expressions []struct {
						Key      string   `json:"key"`
						Operator string   `json:"operator"`
						Values   []string `json:"values"`
					} `json:"matchExpressions"`
				} `json:"nodeSelectorTerms"`
			} `json:"required"`
		} `json:"nodeAffinity"`
	} `json:"spec"`
}

func pvBelongsToNode(pv persistentVolume, nodeID string) bool {
	if pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return false
	}
	for _, term := range pv.Spec.NodeAffinity.Required.Terms {
		for _, expression := range term.Expressions {
			if expression.Key != topologyKey || expression.Operator != "In" {
				continue
			}
			for _, value := range expression.Values {
				if value == nodeID {
					return true
				}
			}
		}
	}
	return false
}

func (d *Driver) reconcileLoop() {
	d.reconcile(context.Background(), time.Now())
	ticker := time.NewTicker(d.config.ReconcileInterval)
	defer ticker.Stop()
	for now := range ticker.C {
		d.reconcile(context.Background(), now)
	}
}

func (d *Driver) reconcile(ctx context.Context, now time.Time) {
	if err := d.checkFence(); err != nil {
		d.log.Error("skipping reconciliation because fence failed", "error", err)
		return
	}
	pvs, err := d.pvs.List(ctx)
	if err != nil {
		d.log.Error("list persistent volumes for reconciliation", "error", err)
		return
	}
	entries, err := os.ReadDir(d.store.volumes)
	if err != nil && !os.IsNotExist(err) {
		d.log.Error("list volume directories", "error", err)
		return
	}
	directories := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		directories[id] = struct{}{}
		if _, exists := pvs[id]; exists {
			d.clearOrphan(id)
			continue
		}
		firstSeen, firstObservation := d.observeOrphan(id, now)
		if firstObservation {
			d.log.Warn("found orphan volume directory", "volume_id", id)
			continue
		}
		if now.Sub(firstSeen) < d.config.OrphanGracePeriod {
			continue
		}
		unlock := d.volumeLocks.lock(id)
		if !d.orphanEligible(id, now) {
			unlock()
			continue
		}
		if err := d.checkFence(); err != nil {
			unlock()
			d.log.Error("skipping orphan deletion because fence failed", "volume_id", id, "error", err)
			continue
		}
		if err := d.store.deleteEntry(id); err != nil {
			unlock()
			d.log.Error("delete orphan volume directory", "volume_id", id, "error", err)
			continue
		}
		d.clearOrphan(id)
		unlock()
		d.log.Info("deleted orphan volume directory", "volume_id", id)
	}
	for id := range pvs {
		if _, exists := directories[id]; !exists {
			d.log.Error("persistent volume directory is missing; it will be recreated empty on publish", "volume_id", id, "path", filepath.Join(d.store.volumes, id))
		}
	}
}

func (d *Driver) observeOrphan(id string, now time.Time) (time.Time, bool) {
	d.orphanMu.Lock()
	defer d.orphanMu.Unlock()
	firstSeen, exists := d.orphanSince[id]
	if !exists {
		d.orphanSince[id] = now
		return now, true
	}
	return firstSeen, false
}

func (d *Driver) orphanEligible(id string, now time.Time) bool {
	d.orphanMu.Lock()
	defer d.orphanMu.Unlock()
	firstSeen, exists := d.orphanSince[id]
	return exists && now.Sub(firstSeen) >= d.config.OrphanGracePeriod
}

func (d *Driver) clearOrphan(id string) {
	d.orphanMu.Lock()
	delete(d.orphanSince, id)
	d.orphanMu.Unlock()
}
