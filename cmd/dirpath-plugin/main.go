package main

import (
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/KeenWill/csi-driver-dirpath/internal/driver"
)

var version = "dev"

func main() {
	endpoint := flag.String("endpoint", "unix:///csi/csi.sock", "CSI endpoint")
	nodeID := flag.String("node-id", "", "node identifier")
	basePath := flag.String("base-path", "/var/lib/dirpath", "path to the prepared backing filesystem")
	fenceToken := flag.String("fence-token", "", "expected .dirpath-fence marker content")
	fenceMode := flag.String("fence-mode", "marker", "fence mode: marker, fsid, or device")
	fenceID := flag.String("fence-id", "", "expected filesystem or device id for strict fence modes")
	orphanReclaim := flag.Bool("orphan-reclaim", true, "delete orphan volume directories")
	orphanGrace := flag.Duration("orphan-grace-period", 10*time.Minute, "time an orphan must remain before deletion")
	reconcileInterval := flag.Duration("reconcile-interval", time.Minute, "orphan reconciliation interval")
	mountMode := flag.String("mount-mode", "real", "mount implementation; noop is for csi-sanity only")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if *nodeID == "" {
		logger.Error("node-id is required")
		os.Exit(2)
	}

	d := driver.New(driver.Config{
		Endpoint:          *endpoint,
		NodeID:            *nodeID,
		Version:           version,
		BasePath:          *basePath,
		FenceToken:        *fenceToken,
		FenceMode:         *fenceMode,
		FenceID:           *fenceID,
		MountMode:         *mountMode,
		OrphanReclaim:     *orphanReclaim,
		OrphanGracePeriod: *orphanGrace,
		ReconcileInterval: *reconcileInterval,
	}, logger)
	if err := d.Run(); err != nil {
		logger.Error("driver stopped", "error", err)
		os.Exit(1)
	}
}
