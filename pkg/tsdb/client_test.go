package tsdb

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	expectedSeries Response[[]model.LabelSet]
	expectedLabels Response[[]string]
)

func testTSDBServer(emptyResponse bool) *httptest.Server {
	// Start test server
	expectedConfig := Response[any]{
		Status: "success",
		Data: map[string]string{
			"yaml": "global:\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  evaluation_interval: 10s\n  external_labels:\n    environment: prometheus-demo\nalerting:\n  alertmanagers:\n  - follow_redirects: true\n    enable_http2: true\n    scheme: http\n    timeout: 10s\n    api_version: v2\n    static_configs:\n    - targets:\n      - demo.do.prometheus.io:9093\nrule_files:\n- /etc/prometheus/rules/*.rules\nscrape_configs:\n- job_name: prometheus\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  static_configs:\n  - targets:\n    - demo.do.prometheus.io:9090\n- job_name: random\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/random.yml\n    refresh_interval: 5m\n- job_name: caddy\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  static_configs:\n  - targets:\n    - localhost:2019\n- job_name: grafana\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  static_configs:\n  - targets:\n    - demo.do.prometheus.io:3000\n- job_name: node\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/node.yml\n    refresh_interval: 5m\n- job_name: alertmanager\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/alertmanager.yml\n    refresh_interval: 5m\n- job_name: cadvisor\n  honor_timestamps: true\n  track_timestamps_staleness: true\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/cadvisor.yml\n    refresh_interval: 5m\n- job_name: blackbox\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  params:\n    module:\n    - http_2xx\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /probe\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  relabel_configs:\n  - source_labels: [__address__]\n    separator: ;\n    regex: (.*)\n    target_label: __param_target\n    replacement: $1\n    action: replace\n  - source_labels: [__param_target]\n    separator: ;\n    regex: (.*)\n    target_label: instance\n    replacement: $1\n    action: replace\n  - separator: ;\n    regex: (.*)\n    target_label: __address__\n    replacement: 127.0.0.1:9115\n    action: replace\n  static_configs:\n  - targets:\n    - http://localhost:9100\n",
		},
	}

	expectedFlags := Response[any]{
		Status: "success",
		Data: map[string]interface{}{
			"query.lookback-delta": "5m",
			"query.max-samples":    "50000000",
			"query.timeout":        "2m",
		},
	}

	expectedRuntimeInfo := Response[any]{
		Status: "success",
		Data: map[string]interface{}{
			"startTime":           "2025-02-18T07:43:52.775090028Z",
			"CWD":                 "/var/lib/prometheus",
			"reloadConfigSuccess": true,
			"lastConfigTime":      "2025-02-18T07:43:53Z",
			"corruptionCount":     0,
			"goroutineCount":      49,
			"GOMAXPROCS":          12,
			"GOMEMLIMIT":          14739915571,
			"GOGC":                "75",
			"GODEBUG":             "",
			"storageRetention":    "30d or 10 GiB",
		},
	}

	expectedSeries = Response[[]model.LabelSet]{
		Status: "success",
		Data: []model.LabelSet{
			{
				"__name__": "up",
				"job":      "prom",
				"instance": "localhost:9090",
			},
			{
				"__name__": "up",
				"job":      "node",
				"instance": "localhost:9091",
			},
			{
				"__name__": "process_start_time_seconds",
				"job":      "prom",
				"instance": "localhost:9090",
			},
		},
	}

	expectedLabels = Response[[]string]{
		Status: "success",
		Data: []string{
			"job", "instance", "__name__",
		},
	}

	expectedQuery := Response[any]{
		Status: "success",
		Data: map[string]interface{}{
			"resultType": "vector",
			"result": []interface{}{
				map[string]interface{}{
					"metric": map[string]string{
						"uuid": "1",
					},
					"value": []interface{}{
						12345, "1.1",
					},
				},
				map[string]interface{}{
					"metric": map[string]string{
						"uuid": "2",
					},
					"value": []interface{}{
						12345, "2.2",
					},
				},
			},
		},
	}

	expectedQueryRange := Response[any]{
		Status: "success",
		Data: map[string]interface{}{
			"resultType": "matrix",
			"result": []interface{}{
				map[string]interface{}{
					"metric": map[string]string{
						"__name__": "up",
						"instance": "localhost:9090",
					},
					"values": []interface{}{
						[]interface{}{1727367964.929, "1"},
						[]interface{}{1727368964.929, "1"},
					},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if emptyResponse {
			expected := Response[any]{
				Status: "error",
			}

			if err := json.NewEncoder(w).Encode(&expected); err != nil {
				w.Write([]byte("KO"))
			}

			return
		}

		if strings.HasSuffix(r.URL.Path, "config") {
			if err := json.NewEncoder(w).Encode(&expectedConfig); err != nil {
				w.Write([]byte("KO"))
			}
		} else if strings.HasSuffix(r.URL.Path, "flags") {
			if err := json.NewEncoder(w).Encode(&expectedFlags); err != nil {
				w.Write([]byte("KO"))
			}
		} else if strings.HasSuffix(r.URL.Path, "runtimeinfo") {
			if err := json.NewEncoder(w).Encode(&expectedRuntimeInfo); err != nil {
				w.Write([]byte("KO"))
			}
		} else if strings.HasSuffix(r.URL.Path, "series") {
			if err := json.NewEncoder(w).Encode(&expectedSeries); err != nil {
				w.Write([]byte("KO"))
			}
		} else if strings.HasSuffix(r.URL.Path, "labels") {
			if err := json.NewEncoder(w).Encode(&expectedLabels); err != nil {
				w.Write([]byte("KO"))
			}
		} else if strings.HasSuffix(r.URL.Path, "query") {
			if err := json.NewEncoder(w).Encode(&expectedQuery); err != nil {
				w.Write([]byte("KO"))
			}
		} else if strings.HasSuffix(r.URL.Path, "query_range") {
			if err := json.NewEncoder(w).Encode(&expectedQueryRange); err != nil {
				w.Write([]byte("KO"))
			}
		}
	}))

	return server
}

func TestNewWithNoURL(t *testing.T) {
	tsdb, err := New("", config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.False(t, tsdb.Available())
}

func TestNewWithURL(t *testing.T) {
	// Start test server
	expected := "dummy data"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(expected))
	}))
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	// Check if Ping is working
	assert.NoError(t, tsdb.Ping())
}

func TestTSDBConfigSuccess(t *testing.T) {
	// Start test server
	server := testTSDBServer(false)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	// Check if Ping is working
	assert.True(t, tsdb.Available())

	// Check settings
	settings, err := tsdb.fetchSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, 15*time.Second, settings.ScrapeInterval)

	// Reduce cache TTL to test concurrent update
	tsdb.settingsCacheTTL = time.Millisecond

	time.Sleep(2 * time.Millisecond)

	expectedSettings := Settings{
		ScrapeInterval:     15 * time.Second,
		EvaluationInterval: 10 * time.Second,
		RateInterval:       60 * time.Second,
		QueryLookbackDelta: defaultLookbackDelta,
		QueryTimeout:       defaultQueryTimeout,
		QueryMaxSamples:    defaultQueryMaxSamples,
		RetentionPeriod:    30 * 24 * time.Hour,
	}

	wg := sync.WaitGroup{}

	// Get settings from concurrent go routines
	for range 10 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			settings := tsdb.Settings(ctx)
			assert.Equal(t, expectedSettings, *settings)
		}()
	}

	wg.Wait()
}

func TestTSDBConfigFail(t *testing.T) {
	// Start test server
	server := testTSDBServer(true)
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if config is working
	_, err = tsdb.fetchSettings(ctx)
	require.Error(t, err)

	// Expected settings with default values
	expectedSettings := Settings{
		ScrapeInterval:     defaultScrapeInterval,
		EvaluationInterval: defaultEvaluationInterval,
		RateInterval:       defaultScrapeInterval * 4,
		QueryLookbackDelta: defaultLookbackDelta,
		QueryTimeout:       defaultQueryTimeout,
		QueryMaxSamples:    defaultQueryMaxSamples,
	}

	settings := tsdb.Settings(ctx)
	assert.Equal(t, expectedSettings, *settings)
}

func TestTSDBSeriesSuccess(t *testing.T) {
	// Start test server
	server := testTSDBServer(false)
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	m, err := tsdb.Series(context.Background(), []string{"up", "process_start_time_seconds"}, time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, expectedSeries.Data, m)
}

func TestTSDBSeriesFail(t *testing.T) {
	/// Start test server
	server := testTSDBServer(true)
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	_, err = tsdb.Series(context.Background(), []string{"up", "process_start_time_seconds"}, time.Time{}, time.Time{})
	assert.Error(t, err)
}

func TestTSDBLabelsSuccess(t *testing.T) {
	// Start test server
	server := testTSDBServer(false)
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	m, err := tsdb.Labels(context.Background(), []string{"up", "process_start_time_seconds"}, time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, expectedLabels.Data, m)
}

func TestTSDBLabelsFail(t *testing.T) {
	// Start test server
	server := testTSDBServer(true)
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	_, err = tsdb.Labels(context.Background(), []string{"up", "process_start_time_seconds"}, time.Time{}, time.Time{})
	assert.Error(t, err)
}

func TestTSDBQuerySuccess(t *testing.T) {
	// Start test server
	server := testTSDBServer(false)
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	m, err := tsdb.Query(context.Background(), "", time.Now())
	require.NoError(t, err)
	assert.Equal(t, Metric{"1": 1.1, "2": 2.2}, m)
}

func TestTSDBQueryFail(t *testing.T) {
	// Start test server
	server := testTSDBServer(true)
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	_, err = tsdb.Query(context.Background(), "", time.Now())
	assert.Error(t, err)
}

func TestTSDBQueryRangeSuccess(t *testing.T) {
	// Start test server
	server := testTSDBServer(false)
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	m, err := tsdb.RangeQuery(context.Background(), "", time.Now(), time.Now(), time.Minute)
	require.NoError(t, err)

	assert.Equal(
		t,
		RangeMetric{
			"up": []model.SamplePair{
				{Timestamp: model.Time(1727367964929), Value: model.SampleValue(1)},
				{Timestamp: model.Time(1727368964929), Value: model.SampleValue(1)},
			},
		},
		m,
	)
}

func TestTSDBQueryRangeFail(t *testing.T) {
	// Start test server
	server := testTSDBServer(true)
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	_, err = tsdb.RangeQuery(context.Background(), "", time.Now(), time.Now(), time.Minute)
	assert.Error(t, err)
}

func TestTSDBDeleteSuccess(t *testing.T) {
	// Start test server
	expected := []string{"metric1", "metric2"}

	var got []string

	var err error

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err = r.ParseForm(); err != nil {
			w.Write([]byte("KO"))

			return
		}

		got = r.Form["match[]"]

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	err = tsdb.Delete(context.Background(), time.Now(), time.Now(), expected)
	require.NoError(t, err)
	assert.ElementsMatch(t, expected, got)
}

func TestTSDBDeleteFail(t *testing.T) {
	// Start test server
	expected := []string{"metric1", "metric2"}

	var err error

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err = r.ParseForm(); err != nil {
			w.Write([]byte("KO"))

			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	err = tsdb.Delete(context.Background(), time.Now(), time.Now(), expected)
	require.Error(t, err)
}
