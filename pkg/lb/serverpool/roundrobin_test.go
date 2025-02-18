package serverpool

import (
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var rrIDs = []string{"rr0", "rr1"}

func TestRoundRobinIteration(t *testing.T) {
	d := 0 * time.Second
	manager, err := New("round-robin", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	// Make dummy backend servers
	backendURLs := make(map[string][]*url.URL, 3)
	backends := make(map[string][]backend.Server, 3)

	for i := range 3 {
		for j, id := range rrIDs {
			backendURL, err := url.Parse(fmt.Sprintf("http://localhost:%d", 3333*(i+1)+j))
			require.NoError(t, err)

			if _, ok := backendURLs[id]; !ok {
				backendURLs[id] = make([]*url.URL, 3)
				backends[id] = make([]backend.Server, 3)
			}

			backendURLs[id][i] = backendURL

			backend, err := backend.NewTSDB(base.ServerConfig{Web: models.WebConfig{URL: backendURL.String()}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			require.NoError(t, err)

			backends[id][i] = backend
			manager.Add(id, backend)
		}
	}

	// Start wait group
	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()

		for range 3 {
			manager.Target(rrIDs[0], d)
		}
	}()

	go func() {
		defer wg.Done()

		for range 2 {
			manager.Target(rrIDs[1], d)
		}
	}()

	// Wait for all go routines
	wg.Wait()

	// This should be backends[0] as next round is multiple of 3 for rrID[0]
	// and backends[2] for rrIDs[1]
	for i, id := range rrIDs {
		assert.Equal(t, backends[id][i].URL().String(), manager.Target(id, d).URL().String())
	}

	// For unknown ID expect nil
	assert.Empty(t, manager.Target("unknown", d))
}
