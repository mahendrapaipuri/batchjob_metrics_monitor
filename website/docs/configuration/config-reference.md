---
sidebar_position: 9
---

# Configuration Reference

The following reference applies to configuration files of CEEMS API server, CEEMS LB and
web configuration. CEEMS uses Prometheus' [client config](https://github.com/prometheus/common/tree/main/config)
to configure HTTP clients. Thus, most of the configuration that is used to configure
HTTP clients resemble that of Prometheus'. The configuration reference has also been
inspired from Prometheus docs.

The file is written in [YAML format](https://en.wikipedia.org/wiki/YAML),
defined by the scheme described below.
Brackets indicate that a parameter is optional. For non-list parameters the
value is set to the specified default.

Generic placeholders are defined as follows:

* `<boolean>`: a boolean that can take the values `true` or `false`
* `<duration>`: a duration matching the regular expression `((([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?|0)`, e.g. `1d`, `1h30m`, `5m`, `10s`
* `<date>`: a date of format `YYYY-MM-DD`
* `<filename>`: a valid path in the current working directory
* `<float>`: a floating-point number
* `<host>`: a valid string consisting of a hostname or IP followed by an optional port number
* `<int>`: an integer value
* `<path>`: a valid URL path
* `<scheme>`: a string that can take the values `http` or `https`
* `<secret>`: a regular string that is a secret, such as a password
* `<string>`: a regular string
* `<size>`: a size in bytes, e.g. `512MB`. A unit is required. Supported units: B, KB, MB, GB, TB, PB, EB.
* `<idname>`: a string matching the regular expression `[a-zA-Z_-][a-zA-Z0-9_-]*`. Any other unsupported
character in the source label should be converted to an underscore
* `<managername>`: a string that identifies resource manager. Currently accepted values are `slurm`.
* `<updatername>`: a string that identifies updater type. Currently accepted values are `tsdb`.
* `<promql_query>`: a valid PromQL query string.
* `<lbstrategy>`: a valid load balancing strategy. Currently accepted values are `round-robin`, `least-connection` and `resource-based`.
* `<object>`: a generic object

The other placeholders are specified separately.

## `<ceems_api_server>`

The following shows the reference for CEEMS API server config.
A valid sample configuration file can be found in the
[repo](https://github.com/mahendrapaipuri/ceems/blob/main/build/config/ceems_api_server/ceems_api_server.yml).

```yaml
# Configuration file to configure CEEMS API server
#
# This config file has following sections:
#  - `ceems_api_server`: Core configuration of CEEMS API server
#  - `clusters`: Configuration of clusters that are being monitored
#  - `updaters`: Configuration of updaters
#
---
# CEEMS API Server and data config
ceems_api_server:
  # Data related configuration of the CEEMS API server. This config concerns with the 
  # locations where data will be saved, frequency of data updates, etc.
  #
  data:
    [ <data_config> ]

  # HTTP web admin related config for CEEMS API server
  #
  admin:
    [ <admin_config> ]

  # HTTP web related config for CEEMS API server.
  #
  web:
    # Maximum allowable query range, ie, the difference between `from` and `to` query 
    # parameters. 
    #
    # This can be used to restrict the query range made by the users to avoid OOM errors
    # when handling too much data.
    #
    # Default value `0s` means no restrictions are imposed on the query.
    #
    # Units Supported: y, w, d, h, m, s, ms.
    #
    [ max_query: <duration> | default: 0s ]

    # Number of requests allowed in ONE MINUTE per client identified by Real IP address.
    # Request headers `True-Client-IP`, `X-Real-IP` and `X-Forwarded-For` are looked up
    # to get the real IP address.
    #
    # This is to effectively impose a rate limit for the entire CEEMS server irrespective
    # of URL path. We advise to set it to a value based on your needs to avoid DoS/DDoS
    # attacks.
    #
    # Rate limiting is done using the Sliding Window Counter pattern inspired by 
    # CloudFlare https://blog.cloudflare.com/counting-things-a-lot-of-different-things/
    #
    # Default value `0` means no rate limiting is applied.
    #
    [ requests_limit: <int> | default: 0 ]

    # It will be used to prefix all HTTP endpoints served by CEEMS API server. 
    # For example, if CEEMS API server is served via a reverse proxy. 
    # 
    # Default is '/'
    #
    [ route_prefix: <path> | default: / ]

# A list of clusters from which CEEMS API server will fetch the compute units.
# 
# Each cluster must provide an unique `id`. The `id` will enable CEEMS to identify 
# different clusters in multi-cluster setup. This `id` must be consistent throughout 
# all the CEEMS components.
# 
clusters:
  [ - <cluster_config> ... ]
  
    
# A list of Updaters that will be used to update the compute unit metrics. This update 
# step can be used to update the aggregate metrics of each compute unit in real time
# or to add complementary information to the compute units from on-premise third 
# party services.
#
# Currently only TSDB updater is supported. The compute unit aggregate metrics can be
# updated from TSDB (Prometheus/VM) instances.
#
updaters:
  [ - <updater_config> ... ]
  
```

### `<data_config>`

A `data_config` allows configuring the DB settings of CEEMS API server.

```yaml
# Path at which CEEMS API server data will be stored.
# If relative path is used, it will be resolved based on the current working directory.
#
[ path: <filename> | default = data ]

# The duration to retain the data in the DB. Units older than this duration will be
# purged from the DB. 
#
# In the case of global usage stats, if the last activity on a given project/user 
# combination is older than this period, those stats will be purged from the DB.
#
# Units Supported: y, w, d, h, m, s, ms.
#
[ retention_period: <duration> | default = 30d ]

# Units data will be fetched at this interval. CEEMS will pull the units from the 
# underlying resource manager at this frequency into its own DB.
#
# Units Supported: y, w, d, h, m, s, ms.
#
[ update_interval: <duration> | default = 15m ]

# Units data will be fetched from this date. If left empty, units will be fetched
# from current day midnight.
#
# Format Supported: 2025-01-01.
#
[ update_from: <date> | default = today ]

# Units data will be fetched at this interval when fetching historical data. For
# example, if `update_from` is set to a date in the past, units will be fetched
# for every `max_update_interval` period until we reach to current time and then
# they will be fetched every `update_interval` time.
#
# Units Supported: y, w, d, h, m, s, ms.
#
[ max_update_interval: <duration> | default = 1h ]

# Time zone to be used when storing times of different events in the DB.
# It takes a value defined in IANA (https://en.wikipedia.org/wiki/List_of_tz_database_time_zones)
# like `Europe/Paris`
# 
# A special value `Local` can be used to use server local time zone.
#
[ time_zone: <string> | default = Local ]

# CEEMS API server is capable of creating DB backups using SQLite backup API. Created
# DB backups will be saved to this path. NOTE that for huge DBs, this backup can take 
# a considerable amount of time. 
#
# Use a different disk device than `ceems_api_server.data.path` to achieve 
# fault tolerance.
#
# If the path is empty, no backups will be created.
#
[ backup_path: <filename> ]

# The interval at which DB back ups will be created. 
#
# Minimum allowable interval is `1d`, ie, 1 day.
#
# Units Supported: y, w, d, h, m, s, ms.
#
[ backup_interval: <duration> | default = 1d ]

```

### `<admin_config>`

A `admin_config` allows configuring the admin users of CEEMS API server.

```yaml
# List of users that will have admin privileges for accessing CEEMS API server
#
# These users will have full access to DB and can query stats of any user/project.
#
# In addition, it is possible to pull users from Grafana teams and add them to 
# admin users. Check `grafana` configuration on how to fetch users from Grafana.
#
users:
    [ - <string> ... ]

# Besides setting a static list of admin users using `ceems_api_server.web.admin_users`,
# it is possible to pull the users from a given Grafana instance and update the admin users
# list of CEEMS API server. This allows operators to add new admins to CEEMS API server
# without having to restart `ceems_api_server`. 
#
# Typically, one or several Grafana team(s) can be created dedicated to CEEMS admins and 
# CEEMS API server will fetch the Grafana team members at the same frequency as compute 
# units.
#
# The web config of Grafana can be set in the following section:
#
grafana:
  [ <grafana_config> ]
```

### `<grafana_config>`

A `grafana_config` allows configuring the Grafana client config to fetch members of
Grafana teams to be added to admin users of CEEMS API server.

```yaml
# Web URL of the Grafana instance
#
url: <host>

# List of IDs of the Grafana teams from which the members will be synchronized 
# with CEEMS admin users
#
teams_ids:
  - <string> ...

# Sets the `Authorization` header on every API request with the
# configured username and password.
# password and password_file are mutually exclusive.
#
basic_auth:
  [ username: <string> ]
  [ password: <secret> ]
  [ password_file: <string> ]

# Sets the `Authorization` header on every API request with
# the configured credentials.
#
authorization:
  # Sets the authentication type of the request.
  [ type: <string> | default: Bearer ]
  # Sets the credentials of the request. It is mutually exclusive with
  # `credentials_file`.
  [ credentials: <secret> ]
  # Sets the credentials of the request with the credentials read from the
  # configured file. It is mutually exclusive with `credentials`.
  [ credentials_file: <filename> ]

# Optional OAuth 2.0 configuration.
# Cannot be used at the same time as basic_auth or authorization.
#
oauth2: 
  [ <oauth2> ]

# Configure whether scrape requests follow HTTP 3xx redirects.
[ follow_redirects: <boolean> | default = true ]

# Whether to enable HTTP2.
[ enable_http2: <boolean> | default: true ]

# Configures the API request's TLS settings.
#
tls_config:
  [ <tls_config> ]

# List of headers that will be passed in the API requests to the server.
# Authentication related headers may be configured in this section. Header name
# must be configured as key and header value supports three different types of 
# headers: values, secrets and files.
#
# The difference between values and secrets is that secret will be redacted
# in server logs where as values will be emitted in the logs.
#
# Values are regular headers with values, secrets are headers that pass secret
# information like tokens and files pass the file content in the headers.
#
http_headers:
  [ <http_headers_config> ]
```

## `<cluster_config>`

A `cluster_config` allows configuring the cluster of CEEMS API server.

```yaml
# Identifier of the cluster. Must be unique for each cluster
#
# Use an id that end users can identify, for instance, name of the cluster.
#
id: <idname>

# Resource manager of the cluster. Currently only `slurm` is supported. In future,
# `openstack` will be supported
#
manager: <managername>

# List of updater IDs to run on the compute units of current cluster. The updaters
# will be run in the same order as provided in the list.
#
# ID of each updater is set in the `updaters` section of the config. If an unknown
# ID is provided here, it will be ignored during the update step.
#
updaters:
  [- <idname> ... ]

# CLI tool configuration.
# 
# If the resource manager supports fetching compute units data from a CLI tool,
# this section can be used to configure the tool. This can be mainly used to configure
# SLURM CLI utility tools that can be used to fetch job data.
#
# When SLURM resource manager is configured to fetch job data using `sacct` command,
# execution mode of the command will be decided as follows:
#
#  - If the current user running `ceems_api_server` is `root`, `sacct`
#    command will be executed as that user in a security context.
# 
#  - If the `ceems_api_server` process has `CAP_SETUID` and `CAP_SETGID` capabilities, `sacct` 
#    command will be executed as `root` user in a security context.
# 
#  - As a last attempt, we attempt to execute `sacct` with `sudo` prefix. If
#    the current user running `ceems_api_server` is in the list of sudoers, this check
#    will pass and `sacct` will be always executed as `sudo sacct <args>` to fetch jobs.
#
# If none of the above conditions are true, `sacct` will be executed as the current user 
# which might not give job data of _all_ users in the cluster.
#
# If the operators are unsure which method to use, there is a default systemd
# unit file provided in the repo that uses Linux capabilities. Use that file as 
# starting point and modify the CLI args accordingly
#
# If no `cli` and no `web` config is found, `ceems_api_server` will check
# if CLI utilities like `sacct` exist on `PATH` and if found, will use them.
#
# Systemd Unit File:
# https://github.com/mahendrapaipuri/ceems/blob/main/build/package/ceems_api_server/ceems_api_server.service
#
cli:
  # Path to the binaries of the CLI utilities.
  #
  [ path: <filename> ]

  # An object of environment variables that will be injected while executing the 
  # CLI utilities to fetch compute unit data. 
  #
  # This is handy when executing CLI tools like `keystone` for openstack or `kubectl` 
  # for k8s needs to source admin credentials. Those credentials can be set manually
  # here in this section. 
  #
  environment_variables: 
    [ <string>: <string> ... ]

# If the resource manager supports API server, configure the REST API
# server details here.
#
# When configured, REST API server is always preferred over CLI utilities for 
# fetching compute units
#
# Most of the web configuration has been inspired from Prometheus `scrape_config`
# and its utility functions are used to create HTTP client using the configuration
# set below.
# 
web:
  # Web client config of resource manager's cluster
  #
  [ <web_client_config> ]

# Any other configuration needed to reach API server of the resource manager
# can be configured in this section.
#
# Currently this section is used for Openstack resource manager
# to configure API versions
#
# In the case of Openstack, this section must have two keys `api_service_endpoints`
# and `auth`. Both of these are compulsory.
# `api_service_endpoints` must provide API endpoints for compute and identity
# services as provided in service catalog of Openstack cluster. `auth` must be the
# same `auth` object that must be sent in POST request to keystone to get a API token.
#
# Example:
#
# extra_config:
#   api_service_endpoints:
#     compute: https://openstack-nova.example.com/v2.1
#     identity: https://openstack-keystone.example.com
#   auth:
#     identity:
#       methods:
#         - password
#       password:
#         user:
#           name: admin
#           password: supersecret
#
extra_config:
  [ <string>: <object> ... ]
```

## `<updater_config>`

A `updater_config` allows configuring updaters of CEEMS API server.

```yaml
# Identifier of the updater. Must be unique for each updater
#
# This identifier should be used in the `updaters` section inside each 
# `clusters` config to update the compute units of that resource manager with a
# given updater.
#
id: <idname>

# Updater kind. Currently only `tsdb` is supported.
#
updater: <updatername>

# Web Config of the updater.
#
web:
  # Web client config of updater instance
  #
  [ <web_client_config> ]

# Any other configuration needed for the updater instance can be configured 
# in this section.
# Currently this section is used for `tsdb` updater to configure the queries that
# will be used to aggregate the compute unit metrics.
#
extra_config:
  # 
  # CEEMS `tsdb` updater makes queries in batches in order to avoid OOM errors on TSDB.
  # The parameters `query_max_series` and `query_min_samples` can be used to
  # control the batch size. 
  #
  # Number of queries that can be loaded into memory depends on `--query.max-samples` 
  # parameter. For a given batch size, all the queries in `queries` section will be
  # executed concurrently. For instance, at a given time, if the batch size is 100 and
  # if there are 40 different series used in `queries` section, the total number of
  # series that will be loaded into the memory will be 100 * 40 = 4000. If the scrape
  # interval is 10s and we are updating for a duration of 60 min, the total number of
  # samples that need to be loaded will be 4000 * (60 * 60) / 10 = 1440000. The default value 
  # used by Prometheus for `--query.max-samples` is 50000000 which is more than
  # what we got in the calculation in the example. However, we need to account for other
  # queries made to the TSDB as well and hence, must leave a good tolerance for all queries
  # to be able to get executed correctly. The updater will fetch the current value of
  # `--query.max-samples` and depending on the provided `query_max_series` and
  # `query_min_samples` config parameters, it estimates a batch size and executes
  # queries in the estimated batch size.
  #
  # Maximum number of series used in `queries` section. If there are 15 different series
  # used in queries, we need to set it to 15. This will be used to
  # estimate batch size when executing queries concurrently.
  #
  # Default value is 50
  #
  [ query_max_series: <int>  | default: 50 ]

  # Minimum number of samples that are guaranteed to available for executing the queries
  # of the updater. It is expressed as proportion of `--query.max-samples` and takes a value
  # between 0 to 1. A smaller value means smaller batch sizes.
  #
  # Default value is 0.5
  #
  [ query_min_samples: <float>  | default: 0.5 ]

  # Compute units that have total life time less than this value will be deleted from 
  # TSDB to reduce number of labels and cardinality
  #
  # Default value `0s` means no compute units will be purged.
  #
  # Units Supported: y, w, d, h, m, s, ms.
  #
  [ cutoff_duration: <duration> | default: 0s ]

  # The ignored units' (based on `cutoff_duration`) metrics will be dropped from the TSDB
  # when set it to `true`. This can be used to reduce number of labels and cardinality of TSDB
  #
  # TSDB must be started with `--web.enable-admin-api` flag for this to work
  #
  [ delete_ignored: <boolean> | default: false ]

  # List of labels to delete from TSDB. These labels should be valid matchers for TSDB
  # More information of delete API of Prometheus https://prometheus.io/docs/prometheus/latest/querying/api/#delete-series
  #
  # TSDB must be started with --web.enable-admin-api flag for this to work
  #
  labels_to_drop:
    [ - <string> ... ]

  # Define queries that are used to estimate aggregate metrics of each compute unit
  # These queries will be passed to golang's text/template package to build them
  # Available template variables
  # - UUIDs -> UUIDs string delimited by "|", eg, 123|345|567
  # - ScrapeInterval -> Scrape interval of TSDB in time.Duration format eg 15s, 1m
  # - ScrapeIntervalMilli -> Scrape interval of TSDB in milli seconds eg 15000, 60000
  # - EvaluationInterval -> Evaluation interval of TSDB in time.Duration format eg 15s, 1m
  # - EvaluationIntervalMilli -> Evaluation interval of TSDB in milli seconds eg 15s, 1m
  # - RateInterval -> Rate interval in time.Duration format. It is estimated based on Scrape interval as 4*scrape_interval
  # - Range -> Duration of interval where aggregation is being made in time.Duration format
  #
  queries:
    [ <queries_config> ]
```

### `<queries_config>`

A `queries_config` allows configuring PromQL queries for TSDB updater of CEEMS API server.

```yaml
#
# It is possible to define multiple "sub-metrics" for each parent metric.
# For instance, for the case of `total_cpu_energy_usage_kwh`, we wish to store
# energy usage from different sources like RAPL, IPMI, we can do so using following
# config:
#
# total_cpu_energy_usage_kwh:
#   rapl_total: <TSDB query to get energy usage from RAPL for the unit>
#   ipmi_total: <TSDB query to get energy usage from IPMI for the unit>
#
# With the above configuration, the server response from API server will contain
# energy usage from both RAPL and IPMI using the same keys as we used in the 
# sub query. For instance, an example response can be:
#
# `{"total_cpu_energy_usage_kwh": {"rapl_total": 100, "ipmi_total": 120}}`
#
# This approach will let the operators to define the metrics freely according to
# their deployments. This will also allow to fetch metrics from third party 
# DBs outside of CEEMS components without hassle.
#
# The placeholder queries shown below should work out-of-the-box with CEEMS 
# exporter and operators are free to deploy more exporters of their own and use
# the metrics from them to estimate aggregated metrics of each compute unit
#
# Average CPU utilisation
#
# Example of valid query:
#
# global:
#   avg_over_time(
#     avg by (uuid) (
#       (
#         rate(ceems_compute_unit_cpu_user_seconds_total{uuid=~"{{.UUIDs}}"}[{{.RateInterval}}])
#         +
#         rate(ceems_compute_unit_cpu_system_seconds_total{uuid=~"{{.UUIDs}}"}[{{.RateInterval}}])
#       )
#       /
#       ceems_compute_unit_cpus{uuid=~"{{.UUIDs}}"}
#     )[{{.Range}}:]
#   ) * 100
avg_cpu_usage:
  [ <string>: <promql_query> ... ]
  

# Average CPU Memory utilisation
#
# Example of valid query:
#
# global:
#   avg_over_time(
#     avg by (uuid) (
#       ceems_compute_unit_memory_used_bytes{uuid=~"{{.UUIDs}}"}
#       /
#       ceems_compute_unit_memory_total_bytes{uuid=~"{{.UUIDs}}"}
#     )[{{.Range}}:]
#   ) * 100
avg_cpu_mem_usage:
  [ <string>: <promql_query> ... ]
  

# Total CPU energy usage in kWh
#
# Example of valid query:
#
# total:
#   sum_over_time(
#     sum by (uuid) (
#       unit:ceems_compute_unit_cpu_energy_usage:sum{uuid=~"{{.UUIDs}}"} * {{.ScrapeIntervalMilli}} / 3.6e9
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
total_cpu_energy_usage_kwh:
  [ <string>: <promql_query> ... ]
  

# Total CPU emissions in gms
#
# Example of valid query:
#
# rte_total:
#   sum_over_time(
#     sum by (uuid) (
#       label_replace(
#         unit:ceems_compute_unit_cpu_energy_usage:sum{uuid=~"{{.UUIDs}}"} * {{.ScrapeIntervalMilli}} / 3.6e9,
#         "common_label",
#         "mock",
#         "hostname",
#         "(.*)"
#       )
#       * on (common_label) group_left ()
#       label_replace(
#         ceems_emissions_gCo2_kWh{provider="rte",country_code="fr"},
#         "common_label",
#         "mock",
#         "hostname",
#         "(.*)"
#       )
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
# emaps_total:
#   sum_over_time(
#     sum by (uuid) (
#       label_replace(
#         unit:ceems_compute_unit_cpu_energy_usage:sum{uuid=~"{{.UUIDs}}"} * {{.ScrapeIntervalMilli}} / 3.6e9,
#         "common_label",
#         "mock",
#         "hostname",
#         "(.*)"
#       )
#       * on (common_label) group_left ()
#       label_replace(
#         ceems_emissions_gCo2_kWh{provider="emaps",country_code="fr"},
#         "common_label",
#         "mock",
#         "hostname",
#         "(.*)"
#       )
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
total_cpu_emissions_gms:
  [ <string>: <promql_query> ... ]
  

# Average GPU utilization
#
# Example of valid query:
#
# global:
#   avg_over_time(
#     avg by (uuid) (
#       DCGM_FI_DEV_GPU_UTIL
#       * on (gpuuuid) group_right ()
#       ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"}
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
avg_gpu_usage:
  [ <string>: <promql_query> ... ]
  

# Average GPU memory utilization
#
# Example of valid query:
#
# global:
#   avg_over_time(
#     avg by (uuid) (
#       DCGM_FI_DEV_MEM_COPY_UTIL
#       * on (gpuuuid) group_right ()
#       ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"}
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
avg_gpu_mem_usage:
  [ <string>: <promql_query> ... ]
  

# Total GPU energy usage in kWh
#
# Example of valid query:
#
# total:
#   sum_over_time(
#     sum by (uuid) (
#       instance:DCGM_FI_DEV_POWER_USAGE:pue_avg * {{.ScrapeIntervalMilli}} / 3.6e9
#       * on (gpuuuid) group_right()
#       ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"}
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
total_gpu_energy_usage_kwh:
  [ <string>: <promql_query> ... ]
  

# Total GPU emissions in gms
#
# Example of valid query:
#
# rte_total:
#   sum_over_time(
#     sum by (uuid) (
#       label_replace(
#         instance:DCGM_FI_DEV_POWER_USAGE:pue_avg * {{.ScrapeIntervalMilli}} / 3.6e+09
#         * on (gpuuuid) group_right ()
#         ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"},
#         "common_label",
#         "mock",
#         "instance",
#         "(.*)"
#       )
#       * on (common_label) group_left ()
#       label_replace(
#         ceems_emissions_gCo2_kWh{provider="rte",country_code="fr"},
#         "common_label",
#         "mock",
#         "instance",
#         "(.*)"
#       )
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
# emaps_total:
#   sum_over_time(
#     sum by (uuid) (
#       label_replace(
#         instance:DCGM_FI_DEV_POWER_USAGE:pue_avg * {{.ScrapeIntervalMilli}} / 3.6e+09
#         * on (gpuuuid) group_right ()
#         ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"},
#         "common_label",
#         "mock",
#         "instance",
#         "(.*)"
#       )
#       * on (common_label) group_left ()
#       label_replace(
#         ceems_emissions_gCo2_kWh{provider="emaps",country_code="fr"},
#         "common_label",
#         "mock",
#         "instance",
#         "(.*)"
#       )
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
total_gpu_emissions_gms:
  [ <string>: <promql_query> ... ]
  

# Total IO write in GB stats
#
# Currently CEEMS exporter do not scrape this metric. Operators can configure
# this metric from third party exporters, if and when available
#
total_io_write_stats:
  [ <string>: <promql_query> ... ]

# Total IO read in GB stats
#
# Currently CEEMS exporter do not scrape this metric. Operators can configure
# this metric from third party exporters, if and when available
#
total_io_read_stats:
  [ <string>: <promql_query> ... ]

# Total ingress traffic stats
#
# Currently CEEMS exporter do not scrape this metric. Operators can configure
# this metric from third party exporters, if and when available
#
total_ingress_stats:
  [ <string>: <promql_query> ... ]

# Total outgress traffic stats
#
# Currently CEEMS exporter do not scrape this metric. Operators can configure
# this metric from third party exporters, if and when available
#
total_outgress_stats:
  [ <string>: <promql_query> ... ]
```

## `<ceems_lb>`

The following shows the reference for CEEMS load balancer config. A valid sample
configuration file can be found in the
[repo](https://github.com/mahendrapaipuri/ceems/blob/main/build/config/ceems_lb/ceems_lb.yml).

```yaml
# Configuration file to configure CEEMS Load Balancer
#
# This config file has following sections:
#  - `ceems_lb`: Core configuration of CEEMS LB
#  - `ceems_api_server`: Client configuration of CEEMS API server
#  - `clusters`: This is optional config which can be used to validate backends IDs
#
---
ceems_lb:
  # Load balancing strategy. Three possibilites
  #
  # - round-robin
  # - least-connection
  # - resource-based
  #
  # Round robin and least connection are classic strategies.
  # Resource based works based on the query range in the TSDB query. The 
  # query will be proxied to the backend that covers the query_range
  #
  [ strategy: <lbstrategy> | default = round-robin ]

  # List of backends for each cluster
  #
  backends:
    [ - <backend_config> ] 
      

# CEEMS API server config.
# This config is essential to enable access control on the TSDB. By excluding 
# this config, no access control is imposed on the TSDB and a basic load balancing
# based on the chosen strategy will be made.
#
# Essentially, basic access control is implemented by checking the ownership of the
# queried unit. Users that belong to the same project can query the units belong
# to that project. 
# 
# For example, if there is a unit U that belongs to User A and 
# Project P. Any user that belongs to same project P can query for the metrics of unit U
# but not users from other projects.
#
ceems_api_server:
  # The DB contains the information of user and projet units and LB will verify
  # if user/project is the owner of the uuid under request to decide whether to
  # proxy request to backend or not.
  #
  # To identify the current user, X-Grafana-User header will be used that Grafana
  # is capable of sending to the datasource. Grafana essenatially adds this header
  # on the backend server and hence it is not possible for the users to spoof this 
  # header from the browser. 
  # In order to enable this feature, it is essential to set `send_user_header = true`
  # in Grafana config file.
  #
  # If both CEEMS API and CEEMS LB is running on the same host, it is preferable to
  # use the DB directly using `data.path` as DB query is way faster than a API request
  # If both apps are deployed on the same host, ensure that the user running `ceems_lb`
  # has permissions to open CEEMS API data files
  #
  data:
    [ <data_config> ]

  # In the case where CEEMS API and ceems LB are deployed on different hosts, we can
  # still perform access control using CEEMS API server by making a API request to
  # check the ownership of the queried unit. This method should be only preferred when
  # DB cannot be access directly as API request has additional latency than querying DB
  # directly.
  #
  # If both `data.path` and `web.url` are provided, DB will be preferred as it has lower
  # latencies.
  #
  web:
    [ <web_client_config> ]
```

### `<backend_config>`

A `backend_config` allows configuring backend TSDB servers for load balancer.

```yaml
# Identifier of the cluster
#
# This ID must match with the ones defined in `clusters` config. CEEMS API server
# will tag each compute unit from that cluster with this ID and when verifying
# for compute unit ownership, CEEMS LB will use the ID to query for the compute 
# units of that cluster.
#
# This identifier needs to be set as header value for `X-Ceems-Cluster-Id` for 
# requests to CEEMS LB to target correct cluster. For instance there are two different 
# clusters, say cluster-0 and cluster-1, that have different TSDBs configured. Using CEEMS 
# LB we can load balance the traffic for these two clusters using a single CEEMS LB 
# deployement. However, we need to tell CEEMS LB which cluster to target for the 
# incoming traffic. This is done via header. 
#
# The TSDBs running in `cluster-0` must be configured on Grafana to send a header
# value `X-Ceems-Cluster-Id` to `cluster-0` in each request. CEEMS LB will inspect
# this header value and proxies the request to correct TSDB in `cluster-0` based
# on chosen LB strategy.
#
id: <idname>

# List of TSDBs for this cluster. Load balancing between these TSDBs will be 
# made based on the strategy chosen.
#
tsdb: 
  [ - <server_config> ]

# List of Pyroscope servers for this cluster. Load balancing between these servers 
# will be made based on the strategy chosen.
#
pyroscope:
  [ - <server_config> ]
```

### `<server_config>`

A `server_config` contains TSDB/Pyroscope server configuration.

```yaml
# Backend server configuration
#
web: <web_client_config>

# A list of labels that must be filtered before proxying
# response back to the client.
#
# This is useful for Openstack and/or k8s case when clients should not
# be able to retrieve compute node or hypervisor related information like
# node address, node name, etc.
#
# All the labels listed here will be filtered from the response before sending
# it to the clients.
#
# IMPORTANT: Currently `filter_labels` is only supported for TSDB backend type.
#
filter_labels: 
  [ - <string> ]
```

## `<web_client_config>`

A `web_client_config` allows configuring HTTP clients.

```yaml
# Web URL of the Grafana instance
#
url: <host>

# Sets the `Authorization` header on every API request with the
# configured username and password.
# password and password_file are mutually exclusive.
#
basic_auth:
  [ username: <string> ]
  [ password: <secret> ]
  [ password_file: <string> ]

# Sets the `Authorization` header on every API request with
# the configured credentials.
#
authorization:
  # Sets the authentication type of the request.
  [ type: <string> | default: Bearer ]
  # Sets the credentials of the request. It is mutually exclusive with
  # `credentials_file`.
  [ credentials: <secret> ]
  # Sets the credentials of the request with the credentials read from the
  # configured file. It is mutually exclusive with `credentials`.
  [ credentials_file: <filename> ]

# Optional OAuth 2.0 configuration.
# Cannot be used at the same time as basic_auth or authorization.
#
oauth2: 
  [ <oauth2> ]

# Configure whether scrape requests follow HTTP 3xx redirects.
[ follow_redirects: <boolean> | default = true ]

# Whether to enable HTTP2.
[ enable_http2: <boolean> | default: true ]

# Configures the API request's TLS settings.
#
tls_config:
  [ <tls_config> ]

# List of headers that will be passed in the API requests to the server.
#
http_headers:
  [ <http_headers_config> ]
```

## `<oauth2>`

OAuth 2.0 authentication using the client credentials grant type. Prometheus fetches an
access token from the specified endpoint with the given client access and secret keys.

```yaml
client_id: <string>
[ client_secret: <secret> ]

# Read the client secret from a file.
# It is mutually exclusive with `client_secret`.
[ client_secret_file: <filename> ]

# Scopes for the token request.
scopes:
  [ - <string> ... ]

# The URL to fetch the token from.
token_url: <string>

# Optional parameters to append to the token URL.
endpoint_params:
  [ <string>: <string> ... ]

# Configures the token request's TLS settings.
tls_config:
  [ <tls_config> ]

# Optional proxy URL.
[ proxy_url: <string> ]
# Comma-separated string that can contain IPs, CIDR notation, domain names
# that should be excluded from proxying. IP and domain names can
# contain port numbers.
[ no_proxy: <string> ]
# Use proxy URL indicated by environment variables (HTTP_PROXY, https_proxy, HTTPs_PROXY, https_proxy, and no_proxy)
[ proxy_from_environment: <boolean> | default: false ]
# Specifies headers to send to proxies during CONNECT requests.
[ proxy_connect_header:
  [ <string>: [<secret>, ...] ] ]
```

## `<tls_config>`

A `tls_config` allows configuring TLS connections.

```yaml
# CA certificate to validate API server certificate with. At most one of ca and ca_file is allowed.
[ ca: <string> ]
[ ca_file: <filename> ]

# Certificate and key for client cert authentication to the server.
# At most one of cert and cert_file is allowed.
# At most one of key and key_file is allowed.
[ cert: <string> ]
[ cert_file: <filename> ]
[ key: <secret> ]
[ key_file: <filename> ]

# ServerName extension to indicate the name of the server.
# https://tools.ietf.org/html/rfc4366#section-3.1
[ server_name: <string> ]

# Disable validation of the server certificate.
[ insecure_skip_verify: <boolean> ]

# Minimum acceptable TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS
# 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
# If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
# See MinVersion in https://pkg.go.dev/crypto/tls#Config.
[ min_version: <string> ]
# Maximum acceptable TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS
# 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
# If unset, Prometheus will use Go default maximum version, which is TLS 1.3.
# See MaxVersion in https://pkg.go.dev/crypto/tls#Config.
[ max_version: <string> ]
```

## `<http_headers_config>`

A `http_headers_config` allows configuring HTTP headers in requests.

```yaml
# Authentication related headers may be configured in this section. Header name
# must be configured as key and header value supports three different types of 
# headers: values, secrets and files.
#
# The difference between values and secrets is that secret will be redacted
# in server logs where as values will be emitted in the logs.
#
# Values are regular headers with values, secrets are headers that pass secret
# information like tokens and files pass the file content in the headers.
#
# Example:
# http_headers:
#   one:
#     values: [value1a, value1b, value1c]
#   two:
#     values: [value2a]
#     secrets: [value2b, value2c]
#   three:
#     files: [testdata/headers-file-a, testdata/headers-file-b, testdata/headers-file-c]
#
[ <string>: 
    values: 
      [- <string> ... ] 
    secrets: 
      [- <secret> ... ]
    files:
      [- <filename> ... ] ... ]
```
