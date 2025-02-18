package serverpool

import (
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func SleepHandler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(3 * time.Second)
}

var h = http.HandlerFunc(SleepHandler)

// req = httptest.NewRequest(http.MethodGet, "/test", nil)
// w   = httptest.NewRecorder()

func TestNew(t *testing.T) {
	for _, strategy := range []string{"round-robin", "least-connection", "resource-based"} {
		m, _ := New(strategy, slog.New(slog.NewTextHandler(io.Discard, nil)))
		b, err := backend.NewTSDB(base.ServerConfig{Web: models.WebConfig{URL: "http://localhost:3333"}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		require.NoError(t, err)

		m.Add("default", b)

		assert.Equal(t, 1, m.Size("default"))
	}
}

func TestNewUnknownStrategy(t *testing.T) {
	_, err := New("unknown", slog.New(slog.NewTextHandler(io.Discard, nil)))
	assert.Error(t, err)
}
