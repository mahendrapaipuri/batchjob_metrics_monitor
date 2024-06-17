// Package resource implements the Fetcher interface that retrieves compute units
// from resource manager
package resource

import (
	"os"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/api/resource"
)

type mockManager struct {
	logger log.Logger
}

const mockResourceManager = "mock"

var (
	macctPath = base.CEEMSServerApp.Flag(
		"mock.acct.path",
		"Absolute path to mock scheduler's accounting executable.",
	).Default("/usr/local/bin/macct").String()
)

func init() {
	// Register manager
	resource.RegisterManager(mockResourceManager, NewMockManager)
}

// Do all basic checks here
func preflightChecks(logger log.Logger) error {
	if _, err := os.Stat(*macctPath); err != nil {
		level.Error(logger).Log("msg", "Failed to open executable", "path", *macctPath, "err", err)
		return err
	}
	return nil
}

// NewMockManager returns a new MockManager that returns compute units
func NewMockManager(cluster models.Cluster, logger log.Logger) (resource.Fetcher, error) {
	err := preflightChecks(logger)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create mock manager.", "err", err)
		return nil, err
	}
	level.Info(logger).Log("msg", "Compute units from mock resource manager will be retrieved.")
	return &mockManager{
		logger: logger,
	}, nil
}

// Add the logic here to get compute units from resource manager and return slice of Unit structs
//
// When making Unit stucts, ensure to format the datetime using base.DatetimeLayout
// Also ensure to set StartTS and EndTS fields to start and end times in unix milliseconds epoch
func (s *mockManager) Fetch(start time.Time, end time.Time) ([]models.ClusterUnits, error) {
	return []models.ClusterUnits{
		{
			Cluster: models.Cluster{
				ID: "mock",
			},
			Units: []models.Unit{
				{
					UUID: "10000",
				},
				{
					UUID: "11000",
				},
			},
		},
	}, nil
}
