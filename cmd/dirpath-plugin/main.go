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
	parsedNodeID := parseFlag(logger, "node-id", *nodeID, driver.ParseNodeID)
	parsedBasePath := parseFlag(logger, "base-path", *basePath, driver.ParseBasePath)
	parsedFenceToken := parseFlag(logger, "fence-token", *fenceToken, driver.ParseFenceToken)
	parsedFenceMode := parseFlag(logger, "fence-mode", *fenceMode, driver.ParseFenceMode)
	parsedMountMode := parseFlag(logger, "mount-mode", *mountMode, driver.ParseMountMode)

	d := driver.New(driver.Config{
		Endpoint:          *endpoint,
		NodeID:            parsedNodeID,
		Version:           version,
		BasePath:          parsedBasePath,
		FenceToken:        parsedFenceToken,
		FenceMode:         parsedFenceMode,
		FenceID:           *fenceID,
		MountMode:         parsedMountMode,
		OrphanReclaim:     *orphanReclaim,
		OrphanGracePeriod: *orphanGrace,
		ReconcileInterval: *reconcileInterval,
	}, logger)
	if err := d.Run(); err != nil {
		logger.Error("driver stopped", "error", err)
		os.Exit(1)
	}
}

func parseFlag[T any](logger *slog.Logger, name, raw string, parse func(string) (T, error)) T {
	value, err := parse(raw)
	if err != nil {
		logger.Error("invalid flag", "flag", name, "error", err)
		os.Exit(2)
	}
	return value
}
