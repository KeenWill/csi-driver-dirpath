package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/KeenWill/csi-driver-dirpath/internal/driver"
)

var version = "dev"

func main() {
	endpoint := flag.String("endpoint", "unix:///csi/csi.sock", "CSI endpoint")
	nodeID := flag.String("node-id", "", "node identifier")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if *nodeID == "" {
		logger.Error("node-id is required")
		os.Exit(2)
	}

	d := driver.New(driver.Config{
		Endpoint: *endpoint,
		NodeID:   *nodeID,
		Version:  version,
	}, logger)
	if err := d.Run(); err != nil {
		logger.Error("driver stopped", "error", err)
		os.Exit(1)
	}
}
