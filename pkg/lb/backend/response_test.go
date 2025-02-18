package backend

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromReverseProxyModifyResponse(t *testing.T) {
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "query") || strings.HasSuffix(r.URL.Path, "query_range") {
			resp := tsdb.Response[tsdb.Data]{
				Data: tsdb.Data{
					Result: []tsdb.Result{
						{
							Metric: map[string]string{
								"job":      "test",
								"instance": "example.com:9010",
								"hostname": "example",
								"status":   "200",
							},
						},
					},
				},
			}

			w.WriteHeader(http.StatusOK)

			if err := json.NewEncoder(w).Encode(&resp); err != nil {
				w.Write([]byte("KO"))
			}
		} else if strings.HasSuffix(r.URL.Path, "series") {
			resp := tsdb.Response[[]map[string]string]{
				Data: []map[string]string{
					{
						"job":      "test",
						"instance": "example.com:9010",
						"hostname": "example",
						"status":   "200",
					},
					{
						"job":      "test",
						"instance": "example.com:9010",
						"hostname": "example",
						"status":   "400",
					},
				},
			}

			w.WriteHeader(http.StatusOK)

			if err := json.NewEncoder(w).Encode(&resp); err != nil {
				w.Write([]byte("KO"))
			}
		} else if strings.HasSuffix(r.URL.Path, "labels") {
			resp := tsdb.Response[[]string]{
				Data: []string{
					"job", "instance", "hostname", "status",
				},
			}

			w.WriteHeader(http.StatusOK)

			if err := json.NewEncoder(w).Encode(&resp); err != nil {
				w.Write([]byte("KO"))
			}
		} else if strings.HasSuffix(r.URL.Path, "values") {
			resp := tsdb.Response[[]string]{
				Data: []string{
					"value1", "value2",
				},
			}

			w.WriteHeader(http.StatusOK)

			if err := json.NewEncoder(w).Encode(&resp); err != nil {
				w.Write([]byte("KO"))
			}
		}
	}))
	defer backendServer.Close()

	rpURL, err := url.Parse(backendServer.URL)
	require.NoError(t, err)

	rproxy := httputil.NewSingleHostReverseProxy(rpURL)
	// rproxy.ErrorLog = log.New(io.Discard, "", 0) // quiet for tests

	// Labels to filter
	labelsToFilter := []string{"instance", "hostname"}
	rproxy.ModifyResponse = PromResponseModifier(labelsToFilter) //nolint:bodyclose // Ref: https://github.com/timakin/bodyclose/issues/11

	frontendProxy := httptest.NewServer(rproxy)
	defer frontendProxy.Close()

	for _, tt := range []string{
		frontendProxy.URL + "/query",
		frontendProxy.URL + "/query_range",
		frontendProxy.URL + "/series",
		frontendProxy.URL + "/labels",
		frontendProxy.URL + "/label/instance/values",
		frontendProxy.URL + "/label/job/values",
	} {
		resp, err := http.Get(tt) //nolint:gosec,noctx
		require.NoError(t, err)

		// Read response body
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		for _, label := range labelsToFilter {
			if strings.Contains(string(b), label) {
				assert.Fail(t, "response for %s contains filtered label %s", tt, label)
			}
		}

		if strings.Contains(tt, "/instance/values") {
			if strings.Contains(string(b), "value1") || strings.Contains(string(b), "value2") {
				assert.Fail(t, "response for %s contains filtered label value", tt)
			}
		} else if strings.Contains(tt, "/job/values") {
			if !strings.Contains(string(b), "value1") || !strings.Contains(string(b), "value2") {
				assert.Fail(t, "response for %s wrongly removed label values", tt)
			}
		}
	}
}
