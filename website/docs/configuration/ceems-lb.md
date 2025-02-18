---
sidebar_position: 4
---

# CEEMS Load Balancer

CEEMS load balancer supports providing load balancer for TSDB and Pyroscope
servers. When both TSDB and Pyroscope backend servers are configured, CEEMS LB
will launch two different web servers listening at two different ports one
for TSDB and one for Pyroscope.

## CEEMS Load Balancer Configuration

CEEMS Load Balancer configuration has one main section and two optional
section. A basic skeleton of the configuration is as follows:

```yaml
# CEEMS Load Balancer configuration skeleton

ceems_lb: <CEEMS LB CONFIG>

# Optional section
ceems_api_server: <CEEMS API SERVER CONFIG>

# Optional section
clusters: <CLUSTERS CONFIG>
```

CEEMS LB uses the same configuration section of `ceems_api_server` and
`clusters` and hence, it is **possible to merge config files** of CEEMS
API server and CEEMS LB. Each component will read the necessary config
from the same file.

A valid sample
configuration file can be found in the
[repo](https://github.com/mahendrapaipuri/ceems/blob/main/build/config/ceems_lb/ceems_lb.yml).

A sample CEEMS LB config file is shown below:

```yaml
ceems_lb:
  strategy: resource-based
  backends:
    - id: slurm-0
      tsdb: 
        - web:
            url: http://localhost:9090
      pyroscope:
        - web:
            url: http://localhost:4040

    - id: slurm-1
      tsdb: 
        - web:
            url: http://localhost:9090

    - id: slurm-2
      pyroscope: 
        - web:
            url: http://localhost:4040
```

- `strategy`: Load balancing strategy. Besides classical `round-robin` and
`least-connection` strategies, a custom `resource-based` strategy is supported.
In the  `resource-based` strategy, the query will be proxied to the TSDB instance
that has the data based on the time period in the query.
- `backends`: A list of objects describing each TSDB backend.
  - `backends[0].id`: It is **important**
     that the `id` in the backend must be the same `id` used in the
     [Clusters Configuration](./ceems-api-server.md#clusters-configuration). This
     is how CEEMS LB will know which cluster to target.
  - `backends[0].tsdb`: A list of TSDB servers that scrape metrics from the
     cluster identified by `id`.
        - `backends[0].tsdb.web`: Client HTTP configuration of TSDB
        - `backends[0].tsdb.filter_labels`: A list of labels to filter before sending
        response to the client. Useful to filter hypervisor or compute node specific
        information for Openstack and k8s clusters.
  - `backends[0].pyroscope`: A list of Pyroscope servers that store profiling data from the
     cluster identified by `id`.
        -`backends[0].pyroscope.web`: Client HTTP configuration of Pyroscope

:::warning[WARNING]

`resource-based` strategy is only supported for TSDB and when used along with
Pyroscope, the load balancing strategy for Pyroscope servers will be defaulted
to `least-connection`.

CEEMS LB is meant to deploy in the same DMZ as the TSDB servers and hence, it
does not support TLS for the backends.

:::

### CEEMS Load Balancer CLI configuration

By default CEEMS LB servers listen at ports `9030` and `9040` when both
TSDB and Pyroscope backend servers are configured. If intended to use
custom ports, the CLI flag `--web.listen-address` must be repeated to set up
port for TSDB and Pyroscope backends. For instance, for the sample config shown
above, the CLI arguments to launch LB servers at custom ports will be:

```bash
ceems_lb --config.file config.yml --web.listen-address ":8000" --web.listen-address ":9000"
```

This will launch TSDB load balancer listening at port `8000` and Pyroscope load
balancer listening at port `9000`.

:::important[IMPORTANT]

When both TSDB and Pyroscope backend servers are configured, the first listen
address is attributed to TSDB and second one to Pyroscope.

:::

### Matching `backends.id` with `clusters.id`

#### Using custom header

This is the tricky part of the configuration which can be better explained with
an example. Consider we are running CEEMS API server with the following
configuration:

```yaml
ceems_api_server:
  data:
    path: /var/lib/ceems
    update_interval: 15m

clusters:
  - id: slurm-0
    manager: slurm
    updaters:
      - tsdb-0
    cli: 
      <omitted for brevity>

  - id: slurm-1
    manager: slurm
    updaters:
      - tsdb-1
    cli: 
      <omitted for brevity>

updaters:
  - id: tsdb-0
    updater: tsdb
    web:
      url: http://tsdb-0
    extra_config:
        <omitted for brevity>
  
  - id: tsdb-1
    updater: tsdb
    web:
      url: http://tsdb-1
    extra_config:
        <omitted for brevity>
```

Here are we monitoring two SLURM clusters: `slurm-0`and `slurm-1`.
There are two different TSDB servers `tsdb-0`
and `tsdb-1` where `tsdb-0` is scrapping metrics from `slurm-0`
and `tsdb-1` scrapping metrics from only `slurm-1`. Assuming
`tsdb-0` is replicating data onto `tsdb-0-replica` and `tsdb-1`
onto `tsdb-1-replica`, we need to use the following config for
`ceems_lb`

```yaml
ceems_lb:
  strategy: resource-based
  backends:
    - id: slurm-0
      tsdb: 
        - web:
            url : http://tsdb-0
        - web:
            url: http://tsdb-0-replica

    - id: slurm-1
      tsdb: 
        - web:
            url: http://tsdb-1
        - web:
            url: http://tsdb-1-replica

```

As metrics data of `slurm-0` only exists in either `tsdb-0` or
`tsdb-0-replica`, we need to set `backends.id` to `slurm-0` for
these TSDB backends.

Effectively we will use CEEMS LB as a Prometheus datasource in
Grafana and while doing so, we need to target correct cluster.
This is done using a custom header `X-Ceems-Cluster-Id`. When
configuring the datasource in Grafana, we need to add `X-Ceems-Cluster-Id`
to the custom headers section and set the value to cluster ID.

For instance, for `slurm-0` cluster the provisioned datasource
config for Grafana will look as follows:

```yaml
- name: CEEMS-TSDB-LB
  type: prometheus
  access: proxy
  url: http://localhost:9030
  basicAuth: true
  basicAuthUser: ceems
  jsonData:
    prometheusVersion: 2.51
    prometheusType: Prometheus
    timeInterval: 30s
    incrementalQuerying: true
    cacheLevel: Medium
    httpHeaderName1: X-Ceems-Cluster-Id
  secureJsonData:
    basicAuthPassword: <ceems_lb_basic_auth_password>
    httpHeaderValue1: slurm-0
```

assuming CEEMS LB is running at port 9030 on the same host as Grafana. Similarly,
for Pyroscope the provisioned config must look like:

```yaml
- name: CEEMS-Pyro-LB
  type: pyroscope
  access: proxy
  url: http://localhost:9040
  basicAuth: true
  basicAuthUser: ceems
  jsonData:
    httpHeaderName1: X-Ceems-Cluster-Id
  secureJsonData:
    basicAuthPassword: <ceems_lb_basic_auth_password>
    httpHeaderValue1: slurm-0
```

Notice that we set the header and value in `jsonData` and `secureJsonData`,
respectively. This ensures that datasource will send the header with
every request to CEEMS LB and then LB will redirect the query request
to correct backend. This allows a single instance
of CEEMS to load balance across different clusters.

#### Using query label

If for any reason, the above strategy does not work for a given deployment,
it is also possible to identify target clusters using query labels. However,
for this strategy to work, it is needed to inject labels to Prometheus metrics.
For example in the above case, using [static_config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#static_config)
we can set a custom label as follows:

```yaml
- job_name: ceems                        
  static_configs:                     
  - targets:                          
    - compute-0:9100          
    labels:                           
      ceems_id: slurm-0
```

CEEMS LB will read value of `ceems_id` label and then redirects the query
to the appropriate backend.

:::important[IMPORTANT]

If both custom header and label `ceems_id` are present in the request to
CEEMS LB, the query label will take the precedence.

:::

Similarly for setting up this label on profiling data in Pyroscope,
it is necessary to use `external_labels` config parameter for Grafana
Alloy when exporting profiles to Pyroscope server. A sample config
for Grafana Alloy that pushes profiling data can be as follows:

```river
pyroscope.write "monitoring" {
  endpoint {
    url = "http://pyroscope:4040"
  }

  external_labels = {
    "ceems_id" = "slurm-0",
  }
}
```

## CEEMS API Server Configuration

This is an optional config when provided will enforce access
control for the backend TSDBs. A sample config file is given
below:

```yaml
ceems_api_server:
  web:
    url: http://localhost:9020
```

- `web.url`: Address at which CEEMS API server is running. CEEMS LB
will make a request to CEEMS API request to verify the ownership of
the comput unit before proxying request to TSDB. All the possible
configuration parameters for `web` can be found in
[Web Client Configuration Reference](./config-reference.md#web_client_config).

If both CEEMS API server and CEEMS LB has access to CEEMS data path,
it is possible to use the `ceems_api_server.db.path` as well to
query the DB directly instead of making an API request. This will have
much lower latency and higher performance.

## Clusters Configuration

Same configuration as discussed in
[CEEMS API Server's Cluster Configuration](./ceems-api-server.md#clusters-configuration)
can be provided as an optional configuration to verify the `backends` configuration.
This is not mandatory and if not provided, CEEMS LB will verify the backend
`ids` by making an API request to CEEMS API server.

## Example configuration files

As it is clear from above sections, there is a lot of common configuration
between CEEMS API server and CEEMS LB. Thus, when it is possible, it is
advised to merge two configurations in one file.

Taking one of the [examples](./ceems-api-server.md#examples) in CEEMS API
server section, we can add CEEMS LB config as follows:

```yaml
ceems_api_server:
  data:
    path: /var/lib/ceems
    update_interval: 15m

  admin:
    users:
      - adm1
  
  web:
    url: http://localhost:9020
    requests_limit: 30

clusters:
  - id: slurm-0
    manager: slurm
    updaters:
      - tsdb-0
    cli: 
      <omitted for brevity>

  - id: os-0
    manager: openstack
    updaters:
      - tsdb-1
    web: 
      <omitted for brevity>

updaters:
  - id: tsdb-0
    updater: tsdb
    web:
      url: http://tsdb-0
    extra_config:
      <omitted for brevity>
  
  - id: tsdb-1
    updater: tsdb
    web:
      url: http://tsdb-1
    extra_config:
      <omitted for brevity>

ceems_lb:
  strategy: resource-based
  backends:
    - id: slurm-0
      tsdb: 
        - web:
            url: http://tsdb-0
        - web:
            url: http://tsdb-0-replica

    - id: os-0
      tsdb: 
        - web:
            url: http://tsdb-1
        - web:
            url: http://tsdb-1-replica
```

This config assumes `tsdb-0` is replicating data to `tsdb-0-replica`,
`tsdb-1` to `tsbd-1-replica` and CEEMS API server is running on
port `9020` on the same host as CEEMS LB.
