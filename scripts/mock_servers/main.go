package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
	"google.golang.org/protobuf/proto"
)

// Default ports.
const (
	pyroPortNum    = ":4040"
	redfishPortNum = ":5000"
	promPortNum    = ":9090"
	osNovaPortNum  = ":8080"
	osKSPortNum    = ":7070"
)

// Regex to capture query.
var (
	queryRegex = regexp.MustCompile("^(.*){")
	regexpUUID = regexp.MustCompile("(?:.+?)[^gpu]uuid=[~]{0,1}\"(?P<uuid>[a-zA-Z0-9-|]+)\"(?:.*)")
)

// hash returns hash of a given string.
func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))

	return h.Sum32()
}

// lenLoop returns number of digits in an integer.
func lenLoop(i uint32) int {
	if i == 0 {
		return 1
	}

	count := 0

	for i != 0 {
		i /= 10
		count++
	}

	return count
}

// ServiceRootHandler handles root of redfish API.
func ServiceRootHandler(w http.ResponseWriter, r *http.Request) {
	if data, err := os.ReadFile("pkg/collector/testdata/redfish/service_root.json"); err == nil {
		w.Write(data)

		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("KO"))
}

// ChassisRootHandler handles chassis collections of redfish API.
func ChassisRootHandler(w http.ResponseWriter, r *http.Request) {
	if data, err := os.ReadFile("pkg/collector/testdata/redfish/chassis_collection.json"); err == nil {
		w.Write(data)

		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("KO"))
}

// ChassisHandler handles a given chassis of redfish API.
func ChassisHandler(w http.ResponseWriter, r *http.Request) {
	chassisID := strings.ReplaceAll(strings.ToLower(r.PathValue("chassisID")), "-", "_")
	if data, err := os.ReadFile(fmt.Sprintf("pkg/collector/testdata/redfish/%s.json", chassisID)); err == nil {
		w.Write(data)

		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("KO"))
}

// ChassisPowerHandler handles chassis power of redfish API.
func ChassisPowerHandler(w http.ResponseWriter, r *http.Request) {
	chassisID := strings.ReplaceAll(strings.ToLower(r.PathValue("chassisID")), "-", "_")
	if data, err := os.ReadFile(fmt.Sprintf("pkg/collector/testdata/redfish/%s_power.json", chassisID)); err == nil {
		w.Write(data)

		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("KO"))
}

// PyroConfigHandler handles pyroscope config.
func PyroConfigHandler(w http.ResponseWriter, r *http.Request) {
	if data, err := os.ReadFile("pkg/lb/testdata/pyroscope/config.yml"); err == nil {
		w.Write(data)

		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("KO"))
}

// ProfilesHandler handles pyroscope profiles.
func ProfilesHandler(w http.ResponseWriter, r *http.Request) {
	// Parse request
	if body, err := io.ReadAll(r.Body); err == nil {
		reqData := querierv1.SelectMergeStacktracesRequest{}
		if err := proto.Unmarshal(body, &reqData); err == nil {
			RespData := &querierv1.SelectMergeStacktracesResponse{
				Flamegraph: &querierv1.FlameGraph{
					Names: []string{reqData.GetLabelSelector()},
				},
			}
			if body, err := proto.Marshal(RespData); err == nil {
				w.Write(body)

				return
			}
		}
	}

	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("KO"))
}

// QueryHandler handles queries.
func QueryHandler(w http.ResponseWriter, r *http.Request) {
	var response tsdb.Response[any]

	var query string

	switch r.Method {
	case http.MethodGet:
		query = r.URL.Query()["query"][0]
	case http.MethodPost:
		// Call ParseForm() to parse the raw query and update r.PostForm and r.Form.
		if err := r.ParseForm(); err != nil {
			http.Error(w, "ParseForm error", http.StatusInternalServerError)

			return
		}

		query = r.FormValue("query")
	default:
		http.Error(w, "Only GET and POST are allowed", http.StatusForbidden)

		return
	}

	// Extract UUIDs from query
	var uuids []string

	uuidMatches := regexpUUID.FindAllStringSubmatch(query, -1)
	for _, match := range uuidMatches {
		if len(match) > 1 {
			for _, uuid := range strings.Split(match[1], "|") {
				// Ignore empty strings
				if strings.TrimSpace(uuid) != "" && !slices.Contains(uuids, uuid) {
					uuids = append(uuids, uuid)
				}
			}
		}
	}

	// Extract only query without any labels
	matches := queryRegex.FindStringSubmatch(query)
	if len(matches) == 2 {
		query = matches[1]
	}

	// log.Println("Query", query, "UUIDs", uuids)

	var results []interface{}

	switch {
	case slices.Contains(
		[]string{
			"avg_cpu_usage", "avg_cpu_mem_usage", "avg_gpu_usage",
			"avg_gpu_mem_usage", "total_cpu_energy_usage_kwh", "total_gpu_energy_usage_kwh",
			"total_cpu_emissions_gms", "total_gpu_emissions_gms",
		}, query):
		// Convert uuid into hash and transform that hash number into float64 between 0 and 100
		for _, uuid := range uuids {
			h := hash(uuid)
			numDigits := lenLoop(h)
			value := float64(h) / math.Pow(10, float64(numDigits)-2)
			results = append(results,
				map[string]interface{}{
					"metric": map[string]string{
						"uuid":     uuid,
						"instance": "localhost:9090",
					},
					"value": []interface{}{
						12345, strconv.FormatFloat(value, 'f', -1, 64),
					},
				})
		}
	case slices.Contains(
		[]string{
			"total_io_read_stats_bytes", "total_io_write_stats_bytes",
		}, query):
		for _, uuid := range uuids {
			h := hash(uuid)
			results = append(results,
				map[string]interface{}{
					"metric": map[string]string{
						"uuid":     uuid,
						"instance": "localhost:9090",
					},
					"value": []interface{}{
						12345, strconv.FormatUint(uint64(h), 10),
					},
				})
		}
	case slices.Contains(
		[]string{
			"total_io_read_stats_requests", "total_io_write_stats_requests",
		}, query):
		for _, uuid := range uuids {
			h := hash(uuid)
			results = append(results,
				map[string]interface{}{
					"metric": map[string]string{
						"uuid":     uuid,
						"instance": "localhost:9090",
					},
					"value": []interface{}{
						12345, strconv.FormatUint(uint64(h)*10, 10),
					},
				})
		}
	case slices.Contains(
		[]string{
			"total_ingress_stats_bytes", "total_outgress_stats_bytes",
		}, query):
		for _, uuid := range uuids {
			h := hash(uuid)
			results = append(results,
				map[string]interface{}{
					"metric": map[string]string{
						"uuid":     uuid,
						"instance": "localhost:9090",
					},
					"value": []interface{}{
						12345, strconv.FormatUint(uint64(h)*100, 10),
					},
				})
		}
	case slices.Contains(
		[]string{
			"total_ingress_stats_packets", "total_outgress_stats_packets",
		}, query):
		for _, uuid := range uuids {
			h := hash(uuid)
			results = append(results,
				map[string]interface{}{
					"metric": map[string]string{
						"uuid":     uuid,
						"instance": "localhost:9090",
					},
					"value": []interface{}{
						12345, strconv.FormatUint(uint64(h)*1000, 10),
					},
				})
		}
	default:
	}
	// responseResults := filterResults(uuids, esults)
	response = tsdb.Response[any]{
		Status: "success",
		Data: map[string]interface{}{
			"resultType": "vector",
			"result":     results,
		},
	}
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		w.Write([]byte("KO"))
	}
}

// ConfigHandler handles Promtheus config.
func ConfigHandler(w http.ResponseWriter, r *http.Request) {
	response := tsdb.Response[any]{
		Status: "success",
		Data: map[string]string{
			"yaml": "global:\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  evaluation_interval: 10s\n  external_labels:\n    environment: prometheus-demo\nalerting:\n  alertmanagers:\n  - follow_redirects: true\n    enable_http2: true\n    scheme: http\n    timeout: 10s\n    api_version: v2\n    static_configs:\n    - targets:\n      - demo.do.prometheus.io:9093\nrule_files:\n- /etc/prometheus/rules/*.rules\nscrape_configs:\n- job_name: prometheus\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  static_configs:\n  - targets:\n    - demo.do.prometheus.io:9090\n- job_name: random\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/random.yml\n    refresh_interval: 5m\n- job_name: caddy\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  static_configs:\n  - targets:\n    - localhost:2019\n- job_name: grafana\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  static_configs:\n  - targets:\n    - demo.do.prometheus.io:3000\n- job_name: node\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/node.yml\n    refresh_interval: 5m\n- job_name: alertmanager\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/alertmanager.yml\n    refresh_interval: 5m\n- job_name: cadvisor\n  honor_timestamps: true\n  track_timestamps_staleness: true\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/cadvisor.yml\n    refresh_interval: 5m\n- job_name: blackbox\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  params:\n    module:\n    - http_2xx\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /probe\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  relabel_configs:\n  - source_labels: [__address__]\n    separator: ;\n    regex: (.*)\n    target_label: __param_target\n    replacement: $1\n    action: replace\n  - source_labels: [__param_target]\n    separator: ;\n    regex: (.*)\n    target_label: instance\n    replacement: $1\n    action: replace\n  - separator: ;\n    regex: (.*)\n    target_label: __address__\n    replacement: 127.0.0.1:9115\n    action: replace\n  static_configs:\n  - targets:\n    - http://localhost:9100\n",
		},
	}
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		w.Write([]byte("KO"))
	}
}

// FlagsHandler handles Promtheus flags.
func FlagsHandler(w http.ResponseWriter, r *http.Request) {
	response := tsdb.Response[any]{
		Status: "success",
		Data: map[string]interface{}{
			"query.lookback-delta": "5m",
			"query.max-samples":    "50000000",
			"query.timeout":        "2m",
		},
	}
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		w.Write([]byte("KO"))
	}
}

// RuntimeInfoHandler handles Promtheus runtime info.
func RuntimeInfoHandler(w http.ResponseWriter, r *http.Request) {
	response := tsdb.Response[any]{
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
			"storageRetention":    "10y",
		},
	}
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		w.Write([]byte("KO"))
	}
}

// LabelNamesHandler handles Promtheus label names.
func LabelNamesHandler(w http.ResponseWriter, r *http.Request) {
	response := tsdb.Response[any]{
		Status: "success",
		Data:   []string{"job", "instance", "__name__"},
	}
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		w.Write([]byte("KO"))
	}
}

// ServersHandler handles OS compute servers.
func ServersHandler(w http.ResponseWriter, r *http.Request) {
	var fileName string
	if _, ok := r.URL.Query()["deleted"]; ok {
		fileName = "deleted"
	} else {
		fileName = "servers"
	}

	if data, err := os.ReadFile(fmt.Sprintf("pkg/api/testdata/openstack/compute/%s.json", fileName)); err == nil {
		w.Write(data)

		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("KO"))
}

// TokensHandler handles OS tokens.
func TokensHandler(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)

	var t map[string]interface{}

	if err := decoder.Decode(&t); err != nil {
		w.Write([]byte("KO"))

		return
	}

	w.Header().Add("X-Subject-Token", "apitokensecret")
	w.WriteHeader(http.StatusCreated)
}

// UsersHandler handles OS users.
func UsersHandler(w http.ResponseWriter, r *http.Request) {
	if data, err := os.ReadFile("pkg/api/testdata/openstack/identity/users.json"); err == nil {
		w.Write(data)

		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("KO"))
}

// ProjectsHandler handles OS projects.
func ProjectsHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if data, err := os.ReadFile(fmt.Sprintf("pkg/api/testdata/openstack/identity/%s.json", userID)); err == nil {
		w.Write(data)

		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("KO"))
}

func redfishProxyTarget(ctx context.Context, i, portNum int, tls bool) {
	log.Println("Starting fake redfish target ", i, " on port", portNum)

	// Registering our handler functions, and creating paths.
	redfishMux := http.NewServeMux()
	redfishMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fmt.Sprintf("BMC for host 192.168.1.%d is running on port %d\n", i, portNum)))
	})

	log.Println("Started Redfish target on port", portNum)
	log.Println("To close connection CTRL+C :-)")

	// Start server
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", portNum),
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           redfishMux,
	}
	defer func() {
		if err := server.Shutdown(ctx); err != nil {
			log.Println("Failed to shutdown fake Redfish target", err)
		}
	}()

	// Spinning up the server.
	var err error
	if tls {
		err = server.ListenAndServeTLS("cmd/redfish_proxy/testdata/certs/localhost.crt", "cmd/redfish_proxy/testdata/certs/localhost.key")
	} else {
		err = server.ListenAndServe()
	}

	if err != nil {
		log.Fatal(err)
	}
}

func redfishServer(ctx context.Context) {
	log.Println("Starting fake redfish server")

	// Registering our handler functions, and creating paths.
	redfishMux := http.NewServeMux()
	redfishMux.HandleFunc("/redfish/v1/", ServiceRootHandler)
	redfishMux.HandleFunc("/redfish/v1/Chassis", ChassisRootHandler)
	redfishMux.HandleFunc("/redfish/v1/Chassis/{chassisID}", ChassisHandler)
	redfishMux.HandleFunc("/redfish/v1/Chassis/{chassisID}/Power", ChassisPowerHandler)

	log.Println("Started Redfish on port", redfishPortNum)
	log.Println("To close connection CTRL+C :-)")

	// Start server
	server := &http.Server{
		Addr:              redfishPortNum,
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           redfishMux,
	}
	defer func() {
		if err := server.Shutdown(ctx); err != nil {
			log.Println("Failed to shutdown fake Redfish server", err)
		}
	}()

	// Spinning up the server.
	err := server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func pyroServer(ctx context.Context) {
	log.Println("Starting fake pyroscope server")

	// Registering our handler functions, and creating paths.
	pyroMux := http.NewServeMux()
	pyroMux.HandleFunc("/querier.v1.QuerierService/SelectMergeStacktraces", ProfilesHandler)
	pyroMux.HandleFunc("/api/v1/status/config", PyroConfigHandler)

	log.Println("Started Pyroscope on port", pyroPortNum)
	log.Println("To close connection CTRL+C :-)")

	// Start server
	server := &http.Server{
		Addr:              pyroPortNum,
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           pyroMux,
	}
	defer func() {
		if err := server.Shutdown(ctx); err != nil {
			log.Println("Failed to shutdown fake Pyroscope server", err)
		}
	}()

	// Spinning up the server.
	err := server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func promServer(ctx context.Context) {
	log.Println("Starting fake prometheus server")

	// Registering our handler functions, and creating paths.
	promMux := http.NewServeMux()
	promMux.HandleFunc("/api/v1/query", QueryHandler)
	promMux.HandleFunc("/api/v1/labels", LabelNamesHandler)
	promMux.HandleFunc("/api/v1/status/config", ConfigHandler)
	promMux.HandleFunc("/api/v1/status/flags", FlagsHandler)
	promMux.HandleFunc("/api/v1/status/runtimeinfo", RuntimeInfoHandler)

	log.Println("Started Prometheus on port", promPortNum)
	log.Println("To close connection CTRL+C :-)")

	// Start server
	server := &http.Server{
		Addr:              promPortNum,
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           promMux,
	}
	defer func() {
		if err := server.Shutdown(ctx); err != nil {
			log.Println("Failed to shutdown fake Prometheus server", err)
		}
	}()

	// Spinning up the server.
	err := server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func osNovaServer(ctx context.Context) {
	log.Println("Starting fake Openstack compute API server")

	// Registering our handler functions, and creating paths.
	osNovaMux := http.NewServeMux()
	osNovaMux.HandleFunc("/v2.1/servers/detail", ServersHandler)

	log.Println("Started Openstack compute API server on port", osNovaPortNum)
	log.Println("To close connection CTRL+C :-)")

	// Start server
	server := &http.Server{
		Addr:              osNovaPortNum,
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           osNovaMux,
	}
	defer func() {
		if err := server.Shutdown(ctx); err != nil {
			log.Println("Failed to shutdown fake Openstack compute API server", err)
		}
	}()

	// Spinning up the server.
	err := server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func osKSServer(ctx context.Context) {
	log.Println("Starting fake Openstack identity API server")

	// Registering our handler functions, and creating paths.
	osKSMux := http.NewServeMux()
	osKSMux.HandleFunc("/v3/auth/tokens", TokensHandler)
	osKSMux.HandleFunc("/v3/users", UsersHandler)
	osKSMux.HandleFunc("/v3/users/{id}/projects", ProjectsHandler)

	log.Println("Started Openstack identity API server on port", osKSPortNum)
	log.Println("To close connection CTRL+C :-)")

	// Start server
	server := &http.Server{
		Addr:              osKSPortNum,
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           osKSMux,
	}
	defer func() {
		if err := server.Shutdown(ctx); err != nil {
			log.Println("Failed to shutdown fake Openstack identity API server", err)
		}
	}()

	// Spinning up the server.
	err := server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	log.Println("Starting fake test servers")

	args := os.Args[1:]

	// Registering our handler functions, and creating paths.
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)

	if slices.Contains(args, "redfish-targets-plain") {
		for i := range 10 {
			go func() {
				redfishProxyTarget(ctx, i, 5000+i, false)
			}()
		}
	}

	if slices.Contains(args, "redfish-targets-tls") {
		for i := range 10 {
			go func() {
				redfishProxyTarget(ctx, i, 5000+i, true)
			}()
		}
	}

	if slices.Contains(args, "redfish") {
		go func() {
			redfishServer(ctx)
		}()
	}

	if slices.Contains(args, "pyro") {
		go func() {
			pyroServer(ctx)
		}()
	}

	if slices.Contains(args, "prom") {
		go func() {
			promServer(ctx)
		}()
	}

	if slices.Contains(args, "os-compute") {
		go func() {
			osNovaServer(ctx)
		}()
	}

	if slices.Contains(args, "os-identity") {
		go func() {
			osKSServer(ctx)
		}()
	}

	sig := <-sigs
	log.Println(sig)

	cancel()

	log.Println("Fake test servers have been stopped")
}
