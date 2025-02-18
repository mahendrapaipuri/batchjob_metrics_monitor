---
sidebar_position: 1
---

# Guide

This section presents a guide to the operators to deploy CEEMS stack. There are two
principal components in the CEEMS stack:

- [CEEMS Exporter](../components/ceems-exporter.md) that must be installed on all
the compute nodes.
- [CEEMS API Server](../components/ceems-api-server.md) that must be installed on a
service node.

Optionally, a third component [CEEMS LB](../components/ceems-lb.md) can be installed on the
same service node as CEEMS API server to enforce access control on metrics.

## Prerequisites

Before starting installation, ensure that the resource managers (SLURM/Openstack) have necessary
[configuration](../configuration/resource-managers.md) to work along with CEEMS exporter and
API server.

### Compute nodes

There are no special requirements for CEEMS exporter to work on compute nodes. Although the
exporter is not extensively tested on different OS distros/architectures, it should work on
all the major distros supported by SLURM/Openstack. The exporter is very light and when all the
[available collectors](../configuration/ceems-exporter.md#collectors) are
enabled on the exporter, it will have a maximum consumption of memory around 150 MB and takes
a CPU time of around 0.05 seconds per scrape request.

If the compute nodes have NVIDIA GPUs, [NVIDIA DCGM](https://docs.nvidia.com/datacenter/dcgm/latest/index.html)
and [NVIDIA DCGM Exporter](https://github.com/NVIDIA/dcgm-exporter/)
must be installed on the compute nodes. Installation instructions of those packages
can be found in their corresponding docs.

Similarly, if the compute nodes have AMD GPUs, [AMD SMI Exporter](https://www.amd.com/en/developer/e-sms/amdsmi-library.html)
must be installed on the compute nodes to get power consumption and performance
metrics of GPUs.

Finally, for the SLURM or k8s clusters, if the continuous profiling of the jobs/pods is required,
[Grafana Alloy](https://grafana.com/docs/alloy/latest/) must be installed on the compute nodes.

### Service node

Different services must be deployed for the CEEMS. They can all be deployed on the
same service node or different nodes. Installing them on a same machine will help
to manage the services easily and reduce the attack surface as all services can be
bound to localhost. The list of required services are:

- Prometheus (compulsory): To scrape metrics from exporters running on compute nodes
- CEEMS API server (compulsory): To store the jobs/VMs data in a standardized DB
- Grafana (compulsory): To construct dashboards to expose metrics for operators and endusers.
- CEEMS LB (optional): To enforce access control to the Prometheus metrics
- Pyroscope (optional): When continuous profiling of SLURM jobs/k8s pods is needed

:::note[NOTE]

The present guide assumes that Prometheus, Pyroscope (if needed) and Grafana are
already installed and configured on the service node. Installation instructions
of each component can be consulted from their respective documentation and hence,
they are omitted here.

:::

Installation instructions for [Prometheus](https://prometheus.io/docs/prometheus/latest/installation/),
[Grafana](https://grafana.com/docs/grafana/latest/setup-grafana/installation/)
and [Pyroscope](https://grafana.com/docs/pyroscope/latest/get-started/) can be found in
their docs. CEEMS API server and CEEMS LB requires very modest system resources
and hence, they can be run alongside Prometheus and Pyroscope on the same service node.
The scaling of this service node must take into account the size of cluster, number
of Prometheus targets, Prometheus data retention period, _etc_. A good recommendation
is to have at least 32 GiB of memory and 8 CPUs which should be enough to host all
the necessary services.

When it comes to the storage, Prometheus works best on local disk storage. Thus,
depending on the required retention period, local SSD/NVMe disks with RAID to achieve
fault tolerance can be a good starting point. There are other options like
[Thanos](https://thanos.io/) and [Cortex](https://cortexmetrics.io/) to
achieve long term storage and fault tolerance for Prometheus data.

## Installation Steps

The installation steps in this section make following assumptions:

- There are two sets of compute nodes: 1 Compute node without GPUs `compute-0` and
1 Compute node with NVIDIA GPUs `compute-gpu-0`.
- A single service node `service-0` is used to install all CEEMS related services

For containerized deployments, `podman` will be used along
with [Quadlet](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html)
to manage container services.

### Installing Exporter(s)

Firstly, all the necessary repositories must be added to the local YUM or DEB repositories.
If local repositories are not maintained, it is possible to download the package files and
install from the package files. The following packages and/or repositories must be added:

- CEEMS Exporter, API Server and Load Balancer RPM and DEB files can be
downloaded from [GH Releases](https://github.com/mahendrapaipuri/ceems/releases/latest).
- When NVIDIA GPUs are present on the cluster [CUDA Repos](https://developer.download.nvidia.com/compute/cuda/repos/)
must be added.

Once all the necessary packages are downloaded and/or added to the repositories, they
can be installed on the compute nodes.

On the compute nodes, following packages must be installed:

<details>
  <summary>RHEL/CentOS/Rockylinux/Alma</summary>

  ```bash
    whoami
    # root

    hostname
    # compute-0 or compute-gpu-0 or service-0

    dnf install ceems_exporter -y
  ```

  When nodes have NVIDIA GPUs, we need to install NVIDIA DCGM and NVIDIA DCGM exporter.

  :::note[NOTE]

  Current guide assumes that NVIDIA driver `>=550` and CUDA `>=12` is available on
  compute nodes

  :::

  ```bash
    whoami
    # root

    hostname
    # compute-gpu-0

    dnf install datacenter-gpu-manager-4-core datacenter-gpu-manager-4-cuda12 datacenter-gpu-manager-4-devel datacenter-gpu-manager-4-proprietary datacenter-gpu-manager-4-proprietary-cuda12 datacenter-gpu-manager-exporter -y
  ```
</details>
<details>
  <summary>Debian/Ubuntu</summary>

  ```bash
    whoami
    # root

    hostname
    # compute-0

    apt-get install ceems_exporter -y
  ```

  When nodes have NVIDIA GPUs, we need to install NVIDIA DCGM and NVIDIA DCGM exporter.

  :::note[NOTE]

  Current guide assumes that NVIDIA driver `>=550` and CUDA `>=12` is available on
  compute nodes

  :::

  ```bash
    whoami
    # root

    hostname
    # compute-gpu-0

    apt-get install datacenter-gpu-manager-4-core datacenter-gpu-manager-4-cuda12 datacenter-gpu-manager-4-devel datacenter-gpu-manager-4-proprietary datacenter-gpu-manager-4-proprietary-cuda12 datacenter-gpu-manager-exporter -y
  ```
</details>

We install `ceems_exporter` on service node `service-0` to export real time and static
emission factor data.

### Configuring Exporter(s)

#### CEEMS Exporter

At minimum, CEEMS exporter must be configured with the CLI arguments that enable the relevant collectors.
This can be done using environment variables which can be provided to systemd service file installed
by the package. For instance, to enable SLURM collector and to disable collector metrics, following must
be added to systemd service file

```bash
whoami
# root

hostname
# compute-0 or compute-gpu-0

cat > /etc/systemd/system/ceems_exporter.service.d/override.conf << EOF
[Service]
Environment=CEEMS_EXPORTER_OPTIONS="--collector.slurm --web.disable-exporter-metrics"
EOF
```

Similarly for Openstack compute nodes, a basic runtime configuration would be as follows:

```bash
whoami
# root

hostname
# compute-0 or compute-gpu-0

cat > /etc/systemd/system/ceems_exporter.service.d/override.conf << EOF
[Service]
Environment=CEEMS_EXPORTER_OPTIONS="--collector.libvirt --web.disable-exporter-metrics"
EOF
```

Optionally, if emissions must be estimated using real time emission factors, we need to
deploy another instance of CEEMS exporter on the service node, `service-0`, to pull the
emission factors and export them to Prometheus. To enable real time emission factors from
[Electricity Maps](https://www.electricitymaps.com/pricing)
and [RTE eCO2 Mix](https://www.rte-france.com/en/eco2mix/co2-emissions), the CLI options
for this exporter must be:

```bash
whoami
# root

hostname
# service-0

cat > /etc/systemd/system/ceems_exporter.service.d/override.conf << EOF
[Service]
Environment=CEEMS_EXPORTER_OPTIONS="--collector.emissions --collector.emissions.provider="rte" --collector.emissions.provider="emaps" --collector.disable-defaults --web.disable-exporter-metrics"
EOF
```

:::warning[WARNING]

Operators need to verify the usage policy of [Electricity Maps](https://www.electricitymaps.com/pricing)
API before using it in their production.

:::

CEEMS package supports static emission factors from historical data provided by
[OWID](https://ourworldindata.org/co2-and-greenhouse-gas-emissions). To estimate emissions using
this static factor, there is no need to deploy the above of CEEMS exporter and emissions will
be estimated directly using the static factor value for a given country.

More details on runtime configuration of CEEMS exporter can be consulted from the
[docs](../configuration/ceems-exporter.md).

By default, no authentication is enabled on CEEMS exporter and it is **strongly recommended** to add
at least the basic authentication. This is done using a web configuration file which is installed by
packages. More details on all the available options for web configuration can be found in its
[dedicated section](../configuration/basic-auth.md).

There is a utility tool `ceems_tool` distributed with CEEMS API server package that can be used to
generate web config file. Assuming `ceems_tool` is available on the current host, web config file
can be generated as follows:

```bash
ceems_tool config create-web-config
```

This command will generate a web config file with basic auth configuration in `config` folder in current
directory named as `web-config.yml`. The config file will only
contain hashed password and the output of the command shows the password in plain text. For example, the
output of above command would be:

```bash
web config file created at config/web-config.yml
plain text password for basic auth is <PASSWORD_WILL_BE DISPLAYED_HERE>
store the plain text password securely as you will need it to configure Prometheus
```

This password must be stored securely to use it when configuring Prometheus. The generated web configuration
file must be placed at `/etc/ceems_exporter/web-config.yml` on the compute nodes.

Finally, CEEMS exporter must be enabled to start at boot and restarted for the changes to take effect.

```bash
whoami
# root

hostname
# compute-0 or compute-gpu-0

systemctl enable ceems_exporter.service
systemctl start ceems_exporter.service
```

For the case of containerized deployments using Podman Quadlets, sample
[systemd Quadlet files](https://github.com/mahendrapaipuri/ceems/tree/main/etc/containers/systemd) are provided
in the repository. Steps to follow to deploy Quadlets:

- Copy [`ceems_exporter.network`](https://github.com/mahendrapaipuri/ceems/blob/main/etc/containers/systemd/ceems_exporter.network)
and [`ceems_exporter.container`](https://github.com/mahendrapaipuri/ceems/blob/main/etc/containers/systemd/ceems_exporter.container) files
to `/etc/containers/systemd` folder.
- Create `/etc/ceems_exporter` folder on the host and copy the generated web configuration file
to `/etc/ceems_exporter/web-config.yml`.
- Modify the `Exec` directive in [`ceems_exporter.container`](https://github.com/mahendrapaipuri/ceems/blob/main/etc/containers/systemd/ceems_exporter.container)
file to add relevant CLI options.
- Execute `systemctl daemon-reload` which should generate necessary service files.
- Finally launch the service using `systemctl start ceems_exporter.service`.

#### DCGM Exporter

DCGM exporter needs a CSV file that lists all the metrics that will be monitored. `datacenter-gpu-manager-exporter`
package installs a default file at `/etc/dcgm-exporter/default-counters.csv` which enables important metrics. Replace
the contents of `default-counters.csv` file with the one
[provided in the CEEMS repo](https://github.com/mahendrapaipuri/ceems/blob/main/etc/nvidia-dcgm-exporter/counters.csv),
which enables more profiling metrics than the default one.

By default DCGM exporter runs without any authentication and it is desirable to run it behind basic auth. DCGM exporter
supports same web configuration file as CEEMS exporter and hence, same web configuration can be used for the both
exporters. Assuming the web configuration file is installed as `/etc/dcgm-exporter/web-config.yml`, it can be passed
to the DCGM exporter using environment variable `DCGM_EXPORTER_WEB_CONFIG_FILE`.

```bash
whoami
# root

hostname
# compute-0 or compute-gpu-0

cat > /etc/systemd/system/nvidia-dcgm-exporter.service.d/override.conf << EOF
[Service]
Environment=DCGM_EXPORTER_WEB_CONFIG_FILE=/etc/dcgm-exporter/web-config.yml
EOF
```

Final step is to enable and start DCGM exporter service.

```bash
whoami
# root

hostname
# compute-0 or compute-gpu-0

systemctl enable nvidia-dcgm-exporter.service
systemctl start nvidia-dcgm-exporter.service
```

:::important[IMPORTANT]

To deploy DCGM exporter as Podman container, ensure the version of Podman is `> 4.3`.
We need to ensure to install [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html)
before deploying DCGM exporter container. For Podman, Container Device Interface (CDI)
must be configured and more details can be found in the
[NVIDIA CDI Docs](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/cdi-support.html).

:::

For the case of containerized deployments using Podman Quadlets, sample
[systemd Quadlet files](https://github.com/mahendrapaipuri/ceems/tree/main/etc/containers/systemd) are provided
in the repository. Steps to follow to deploy Quadlets:

- Copy [`nvidia-dcgm-exporter.container`](https://github.com/mahendrapaipuri/ceems/blob/main/etc/containers/systemd/nvidia-dcgm-exporter.container)
file to `/etc/containers/systemd` folder.
- Create `/etc/dcgm-exporter` folder on the host and copy the generated web configuration file
to `/etc/dcgm-exporter/web-config.yml` and [counters.csv](https://github.com/mahendrapaipuri/ceems/blob/main/etc/nvidia-dcgm-exporter/counters.csv)
to `/etc/dcgm-exporter/default-counters.csv`.
- Execute `systemctl daemon-reload` which should generate necessary service files.
- Finally launch the service using `systemctl start nvidia-dcgm-exporter.service`.

### Configuring Prometheus

Assuming Prometheus has already been installed on `service-0`, following scrape configuration must be added to
Prometheus. Remember that in the current deployment scenario, we have two sets of compute nodes:

- 1 Compute node without GPUs `compute-0`
- 1 Compute node with NVIDIA GPUs `compute-gpu-0`
- 1 Service node where emission factors are fetched and exported

We define three different scrape jobs: `cpu-nodes`, `gpu-nodes` and `service-nodes`
to set up CEEMS exporter targets. We can either add DCGM exporter targets in `gpu-nodes`
job or define a separate scrape job for DCGM exporter. In the current scenario,
we setup DCGM exporters in the same job.

:::note[NOTE]

We will need the plain text basic auth password generated for CEEMS and DCGM exporters in the
previous step to configure Prometheus scrape jobs.

:::

The scrape jobs configuration would be as follows:

```yml
# A list of scrape configurations.
scrape_configs:
  - job_name: cpu-nodes
    scheme: http
    metrics_path: /metrics
    basic_auth:
      username: ceems
      password: <BASIC_AUTH_PLAIN_TEXT_PASSWORD>
    static_config:
      targets:
        - compute-0:9010

  - job_name: gpu-nodes
    scheme: http
    metrics_path: /metrics
    basic_auth:
      username: ceems
      password: <BASIC_AUTH_PLAIN_TEXT_PASSWORD>
    # This relabel_config must be added to all
    # scrape jobs that have DCGM targets
    metric_relabel_configs:
       source_labels:
          - modelName
          - UUID
        target_label: gpuuuid
        regex: NVIDIA(.*);(.*)
        replacement: $2
        action: replace
      - source_labels:
          - modelName
          - GPU_I_ID
        target_label: gpuiid
        regex: NVIDIA(.*);(.*)
        replacement: $2
        action: replace
      - regex: UUID
        action: labeldrop
      - regex: GPU_I_ID
        action: labeldrop
    static_config:
      targets:
        - compute-gpu-0:9010
        - compute-gpu-0:9400

  # This job is needed only when exporter is deployed
  # on service node to pull real time emission factors
  # from RTE eCo2 mix and/or Electricity Maps
  - job_name: service-nodes
    scheme: http
    metrics_path: /metrics
    basic_auth:
      username: ceems
      password: <BASIC_AUTH_PLAIN_TEXT_PASSWORD>
    static_config:
      targets:
        - service-0:9010
```

:::important[IMPORTANT]

All the Prometheus scrape jobs that have DCGM exporter targets must include a
`metric_relabel_configs` as follows:

```yml
metric_relabel_configs:
  - source_labels:
      - modelName
      - UUID
    target_label: gpuuuid
    regex: NVIDIA(.*);(.*)
    replacement: $2
    action: replace
  - source_labels:
      - modelName
      - GPU_I_ID
    target_label: gpuiid
    regex: NVIDIA(.*);(.*)
    replacement: $2
    action: replace
  - regex: UUID
    action: labeldrop
  - regex: GPU_I_ID
    action: labeldrop
```

:::

This is only basic configuration and more options can be found in the
[Prometheus](https://prometheus.io/docs/prometheus/latest/configuration/configuration)
configuration docs. Once this configuration has been added,
[reload](https://prometheus.io/docs/prometheus/latest/management_api/#reload) Prometheus and
check if it is able to scrape the targets. This can be verified using Prometheus
Web UI.

Once the Prometheus is able to scrape targets and ingest metrics, we will need to add
[recording rules](https://prometheus.io/docs/practices/rules/) to create new derived metrics
from the raw metrics exported by the CEEMS and DCGM exporters. The advantage of using
recording rules is that Prometheus will calculate these metrics at ingest time once and
there is no need to make calculation each time we want to make queries.

Recording rules can be created using `ceems_tool` using the following command:

```bash
./bin/ceems_tool tsdb create-recording-rules --url=http://<PROMETHEUS_BASIC_AUTH_USERNAME>:<PROMETHEUS_BASIC_AUTH_PASSWORD>@service-0:9090 --country-code=FR
```

:::important[IMPORTANT]

When [Redfish Collector](../components/ceems-exporter.md#energy-related-collectors) is enabled
on CEEMS exporters and if Redfish server has multiple chassis defined, the above command will ask
for the user input on which chassis must be used in estimated power consumption. As different chassis
can report power consumption of different components, operators must choose a chassis that reports
power consumption of host.

:::

The `--url` must be URL at which Prometheus server is running and `--country-code` must be
ISO2 country code to get the emission factor. This command will generate recording rules
files in current directory inside a folder named `rules`. Copy these rules files to
`/etc/prometheus/rules` directory and ensure to set following configuration for Prometheus:

```yml
rule_files:
  - /etc/prometheus/rules/*.rules
```

Reload Prometheus and verify the rules are being evaluted and recorded correctly.

### Installing and Configuring CEEMS API Server

Before going to this step, ensure that Prometheus is able to scrape the targets well
and all the metrics are being monitored.

CEEMS API server can be installed on the same host as Prometheus or a different one.
In the current example, we use the same host as Prometheus for simplicity. Assuming
Prometheus has been installed on service node `service-0`, CEEMS API server can
be installed as follows:

<details>
  <summary>RHEL/CentOS/Rockylinux/Alma</summary>

  ```bash
    whoami
    # root

    hostname
    # service-0

    dnf install ceems_api_server -y
  ```
</details>
<details>
  <summary>Debian/Ubuntu</summary>

  ```bash
    whoami
    # root

    hostname
    # service-0

    apt-get install ceems_api_server -y
  ```
</details>

CEEMS API server stores all the data related to compute units and hence, it is
**strongly** recommended to protect the server using authentication. It supports
the same authentication mechanism as CEEMS and DCGM exporter as explained in the
[previous section](#configuring-exporters). `ceems_tool` can be leveraged to
generate a web configuration file. Copy the generated configuration file to
`/etc/ceems_api_server/web-config.yml`.

Now CEEMS API server config must be updated. `ceems_api_server` package installs
a default configuration file at `/etc/ceems_api_server/config.yml` with sane
defaults. More details about configuration parameters can be consulted in the
[Configuration Reference](../configuration/config-reference.md). Here we need to
add configuration for `clusters` and `updaters` sections in the file.

First, we will start with `updaters` section. `updaters` are list of servers that
will be used to estimate aggregate metrics of each compute unit and store them in
a SQL DB. For example in the current scenario, Prometheus server is an updater from
which we can estimate the aggregate metrics of compute units. The advantage of using
updaters is that we do not need to make expensive repeated queries to Prometheus to
get aggregate values of the metrics.

In order to estimate aggregated metrics, we need to configure the updater with TSDB
queries that estimates aggregated metrics. Assuming that recording rules for Prometheus
are added as explained in the [Configuring Prometheus](#configuring-prometheus) section,
we can generate the queries needed for the updater using `ceems_tool` as follows:

```bash
ceems_tool tsdb create-ceems-tsdb-updater-queries --url=http://<PROMETHEUS_BASIC_AUTH_USERNAME>:<PROMETHEUS_BASIC_AUTH_PASSWORD>@service-0:9090
```

The `--url` must point to Prometheus URL and above command will output the `queries`
configuration section to the terminal. Copy this output and store it on clipboard. Every
updater must have an unique identifier and assuming `prom-tsdb` as identifier and
`QUERIES_OUTPUT` as the queries returned by the above command, following
configuration must be added to `updaters` section in file `/etc/ceems_api_server/config.yml`:

```yml
updaters:
  - id: prom-tsdb
    updater: tsdb
    web:
      url: http://service-0:9090
      basic_auth:
        username: <PROMETHEUS_BASIC_AUTH_USERNAME>
        password: <PROMETHEUS_BASIC_AUTH_PASSWORD>
    extra_config:
      queries: <QUERIES_OUTPUT>
```

Finally, we need to configure `clusters` section in the configuration file. `clusters`
section defines the list of clusters from where we fetch computt units data. It can be
multiple clusters of same kind or multiple clusters of different kind. Each cluster
must be identified by a unique identifier like in the case of `updater`.

We assume that the resource manager of the current scenario is SLURM. In this case,
the host where CEEMS API server will be deployed must be configured as a SLURM client
to be able to execute `sacct` command to get list of jobs. Assuming that it has been
done, the `clusters` section in file `/etc/ceems_api_server/config.yml` must have following
configuration:

```yml
clusters:
  - id: slurm-cluster
    manager: slurm
    # Updater id that we defined in the `updaters` section
    # Aggregate metrics of job will be estimated by querying
    # against this Prometheus server
    updaters:
      - prom-tsdb
    # If `sacct` command is installed in a non-standard location,
    # set the path here
    cli:
      path: /usr/bin
```

With the above `clusters` and `updaters` configurations in-place in
`/etc/ceems_api_server/config.yml`, we can enable and start the CEEMS API server

```bash
systemctl enable ceems_api_server.service
systemctl start ceems_api_server.service
```

Once the API server has started, we can check for its health by hitting endpoint
`http://localhost:9020/api/v1/health` assuming we are on the host where API server
has been deployed.

Once the Prometheus and CEEMS API server are up and running, we can configure Grafana
to use these servers as data sources for building dashboards.

### Configuring Grafana

The final step of the deployment guide is to configure Grafana to use Prometheus and
CEEMS API server has datasources to build dashboards. Assuming Grafana server is also
installed on the same service node `service-0`, firstly, we need to ensure that
Grafana server is configured to send the user header to datasources. This can be done
using the following configuration in `grafana.ini` file:

```ini
[dataproxy]
send_user_header = true
```

or setting `GF_DATAPROXY_SEND_USER_HEADER=true` environment variable on Grafana server.

Following we need to install [Grafana Infinity Datasource](https://grafana.com/docs/plugins/yesoreyeram-infinity-datasource/latest/)
plugin using following command:

```bash
grafana-cli plugins install yesoreyeram-infinity-datasource
```

Once the plugin has been installed, restart Grafana server.

:::important[IMPORTANT]

For the plugin versions `yesoreyeram-infinity-datasource < 3.x`, plugin does not support
`X-Grafana-User` header which we need for CEEMS API server to identify current user. So,
it is recommended to use version `>= 3.x`.

:::

We use Grafana [provisioning](https://grafana.com/docs/grafana/latest/administration/provisioning/)
to define the datasources. A sample
[provisioned datasources](https://github.com/mahendrapaipuri/ceems/blob/main/etc/grafana/provisioning/datasources/ceems.yml)
file is provided in the repository. For the current scenario, the provisioning file would be as follows:

```yml
# Configuration file version
apiVersion: 1

# List of datasources that CEEMS uses
datasources:
  # Vanilla Prometheus datasource that DOES NOT IMPOSE ANY ACCESS CONTROL
  - name: prom
    type: prometheus
    access: proxy
    # Replace it with Prometheus URL
    url: <PROMETHEUS_URL>
    basicAuth: true
    # Replace it with Prometheus basic auth username
    basicAuthUser: <PROMETHEUS_BASIC_AUTH_USERNAME>
    secureJsonData:
      # Replace it with Prometheus basic auth password
      basicAuthPassword: <PROMETHEUS_BASIC_AUTH_PASSWORD>

  # CEEMS API server JSON datasource
  - name: ceems-api
    type: yesoreyeram-infinity-datasource
    url: <CEEMS_API_SERVER_URL>
    basicAuth: true
    # Replace it with CEEMS API server basic auth username
    basicAuthUser: <CEEMS_API_SERVER_BASIC_AUTH_USERNAME>
    jsonData:
      auth_method: basicAuth
      timeout: 120
      # Replace it with CEEMS API server URL
      allowedHosts:
        - <CEEMS_API_SERVER_URL>
      httpHeaderName1: X-Grafana-User
    secureJsonData:
      # Replace it with CEEMS API server basic auth password
      basicAuthPassword: <CEEMS_API_SERVER_BASIC_AUTH_PASSWORD>
      # This will be replaced by username before passing to API server
      # This feature is available only for yesoreyeram-infinity-datasource >= 3.x
      # IMPORTANT: Need $$ to escape $
      httpHeaderValue1: $${__user.login}
```

Replace the placeholders with values and install the file at `/etc/grafana/provisioning/datasources`.
Now restarting the Grafana must include all the newly provisioned datasources.

The next step is to setup dashboards to visualize the metrics of compute units.
This can be done using Grafana provisioning as well. A reference set of dashboards
are provided in the [repository](https://github.com/mahendrapaipuri/ceems/tree/main/thirdparty/grafana).
More details on the dashboards are provided in the
[README](https://github.com/mahendrapaipuri/ceems/blob/main/thirdparty/grafana/README.md).

## Optional Steps

Note that with above installation steps, a functional CEEMS deployment can be assured. However,
if access control to Prometheus data must be enforced, an additional component
[CEEMS LB](../components/ceems-lb.md) must be also deployed. In a nutshell this component sits
between Grafana and Prometheus to introspect the queries coming from Grafana to verify if the
user making the query has view access to the metrics of the compute unit they are querying for.

As discussed in the [Prerequisites](#prerequisites), in order to enable continuous profiling
of SLURM jobs or k8s pods, Grafana Alloy must be installed on compute nodes and Pyroscope
must be installed on service node.

### Deploying Grafana Alloy and Pyroscope

Firstly, ensure that [Grafana Alloy](https://grafana.com/docs/alloy/latest/set-up/install/linux/)
and [Pyroscope](https://github.com/grafana/pyroscope/releases) packages must be added and enabled.

First we must install Pyroscope server so that Grafana Alloy running on compute nodes
can send profile data to Pyroscope. We deploy Pyroscope on the service node `service-0`:

<details>
  <summary>RHEL/CentOS/Rockylinux/Alma</summary>

  ```bash
    whoami
    # root

    hostname
    # service-0

    dnf install pyroscope -y
  ```
</details>
<details>
  <summary>Debian/Ubuntu</summary>

  ```bash
    whoami
    # root

    hostname
    # service-0

    apt-get install pyroscope -y
  ```
</details>

A basic configuration file is provided
in the [repository](https://github.com/mahendrapaipuri/ceems/tree/main/etc/pyroscope)
and it can be used as a good starting point. It must be installed at `/etc/pyroscope/config.yml`.
More details on Pyroscope configuration can be
found in the [documentation](https://grafana.com/docs/pyroscope/latest/configure-server/).

:::note[NOTE]

It is highly recommended to configure [TLS auth](https://grafana.com/docs/pyroscope/latest/configure-server/reference-configuration-parameters/#server)
for Pyroscope to enforce authentication. If managing TLS certificates is not desired, we
recommended to use basic auth by exposing Pyroscope behind a reverse proxy like nginx and
configuring the nginx server block with basic auth credentials. In the absence of any form
of authentication, end users in a typical HPC environment will be able to query Pyroscope
server directly which is not desired.

:::

On the compute nodes, following packages must be installed:

<details>
  <summary>RHEL/CentOS/Rockylinux/Alma</summary>

  ```bash
    whoami
    # root

    hostname
    # compute-0 or compute-gpu-0

    dnf install alloy -y
  ```
</details>
<details>
  <summary>Debian/Ubuntu</summary>

  ```bash
    whoami
    # root

    hostname
    # compute-0 or compute-gpu-0

    apt-get install alloy -y
  ```
</details>

A sample configuration file is provided in the
[repository](https://github.com/mahendrapaipuri/ceems/blob/main/etc/alloy/config.alloy).
Necessary placeholders on the sample config file must be replaced and file
must be installed at `/etc/alloy/config.alloy`.

We need to enable Grafana Alloy targets discoverer component on CEEMS exporter
so that it provides a list of targets to profile to Grafana Alloy. This can be
done by configuring `CEEMS_EXPORTER_OPTIONS` environment variable for CEEMS exporter
service:

```bash
whoami
# root

hostname
# compute-0 or compute-gpu-0

cat > /etc/systemd/system/ceems_exporter.service.d/override.conf << EOF
[Service]
Environment=CEEMS_EXPORTER_OPTIONS="--collector.slurm --collector.alloy-targets --web.disable-exporter-metrics"
EOF
```

Finally, enable and restart both CEEMS Exporter and Grafana Alloy services:

```bash
whoami
# root

hostname
# compute-0 or compute-gpu-0

systemctl enable ceems_exporter.service
systemctl restart ceems_exporter.service

systemctl enable alloy.service
systemctl restart alloy.service
```

:::tip[TIP]

If Grafana Alloy throws any errors, ensure that `alloy.service` is running as `root`
user in the systemd service file. Grafana Alloy needs to access a lot of files in
`/proc` and `/sys` file systems to be able to continuously profile processes which
are otherwise not permitted for non-privileged users.

:::

After this step, Grafana Alloy should be sending the profiles data to Pyroscope for every
SLURM job on the compute node.

### Installing and Configuring CEEMS LB

Before going to this step, ensure that CEEMS API server, Prometheus and Grafana are
installed, configured and working as expected.

CEEMS LB can be installed on the same host as Prometheus. It is more practical ans secure
to deploy it on the same node where Prometheus is running. It is a simple proxy/load balancer
that does not need a lot of resources.

In the current example, we use the same host as Prometheus for simplicity. Assuming
Prometheus has been installed on service node `service-0`, CEEMS LB can
be installed as follows:

<details>
  <summary>RHEL/CentOS/Rockylinux/Alma</summary>

  ```bash
    whoami
    # root

    hostname
    # service-0

    dnf install ceems_lb -y
  ```
</details>
<details>
  <summary>Debian/Ubuntu</summary>

  ```bash
    whoami
    # root

    hostname
    # service-0

    apt-get install ceems_lb -y
  ```
</details>

Just like CEEMS exporter and API server, it is
**strongly** recommended to protect the load balancer using authentication. It supports
the same authentication mechanism as other components as explained in the
[previous section](#configuring-exporters). `ceems_tool` can be leveraged to
generate a web configuration file. Copy the generated configuration file to
`/etc/ceems_lb/web-config.yml`.

Now CEEMS LB config must be updated. `ceems_lb` package installs
a default configuration file at `/etc/ceems_lb/config.yml` with sane
defaults. More details about configuration parameters can be consulted in the
[Configuration Reference](../configuration/config-reference.md). The core configuration
for CEEMS LB is simple and it takes two keys `strategy` and `backends`. `strategy` is
the load balancing strategy whereas `backends` is the list of TSDB (and/or Pyroscope)
backends.

The true value that CEEMS LB offers is the ability to provide access control to
Prometheus query data. Deploying CEEMS LB without access control enabled is not very
useful and not recommended. In order to enable access control an additional section
`ceems_api_server` must be provided to CEEMS LB config. This section must provide
either the `ceems_api_server.data` section or `ceems_api_server.web` section. If
CEEMS LB is able to access the DB files of CEEMS API server, it is recommended to setup
`ceems_api_server.data.path` file so that CEEMS LB will make queries directly to
the DB. If CEEMS API server's DB files are not available to CEEMS LB, it will make HTTP
requests to CEEMS API server to verify the ownership of compute units. It should be
preferred to gives direct access to DB files to CEEMS LB to maximize performance and
minimize latencies.

In the current scenario, as both CEEMS API server and CEEMS LB are deployed on the
same physical host, we use `ceems_api_server.data.path` method for DB access. The
configuration file would be as follows:

```yml
ceems_lb:
  # Load balancing strategy
  strategy: resource-based

  # List of Prometheus and/or Pyroscope backends
  backends:
      # `id` should be the same as configured in `clusters` config.
    - id: slurm-cluster
      tsdb: 
        - web:
            url: http://<PROMETHEUS_URL>
            basic_auth:
              username: <PROMETHEUS_BASIC_AUTH_USERNAME>
              password: <PROMETHEUS_BASIC_AUTH_PASSWORD>
      
      # When Pyroscope is also deployed
      pyroscope:
        - web:
            url: <PYROSCOPE_URL>

# Must be same config as configured for `ceems_api_server` at `/etc/ceems_api_server/config.yml`
ceems_api_server:
  data: /var/lib/ceems
```

Replace the configuration file's content at `/etc/ceems_lb/config.yml` with above file after replacing
placeholders and restart the CEEMS LB service.

```bash
whoami
# root

hostname
# service-0

systemctl enable ceems_lb.service
systemctl start ceems_lb.service
```

This should ensure that CEEMS LB running at `localhost:9030`. When Pyroscope server has also been
deployed and configured in `ceems_lb.backends`, we will notice another HTTP server running at
`localhost:9040`. Normally, server running at `localhost:9030` is load balancer for Prometheus
backends where server running at `localhost:9040` is load balancer for Pyroscope backends. This
can be confirmed by looking at the logs of the `ceems_lb`.

```bash
time=2025-02-13T16:43:51.775Z level=INFO source=frontend.go:220 msg="Starting ceems_lb" backend_type=pyroscope listening=127.0.0.1:9040
time=2025-02-13T16:43:51.775Z level=INFO source=tls_config.go:347 msg="Listening on" backend_type=pyroscope address=127.0.0.1:9040
time=2025-02-13T16:43:51.775Z level=INFO source=tls_config.go:350 msg="TLS is disabled." backend_type=pyroscope http2=false address=127.0.0.1:9040
time=2025-02-13T16:43:51.775Z level=INFO source=helpers.go:55 msg="Starting health checker" backend_type=tsdb
time=2025-02-13T16:43:51.775Z level=INFO source=frontend.go:220 msg="Starting ceems_lb" backend_type=tsdb listening=127.0.0.1:9030
time=2025-02-13T16:43:51.776Z level=INFO source=tls_config.go:347 msg="Listening on" backend_type=tsdb address=127.0.0.1:9030
time=2025-02-13T16:43:51.776Z level=INFO source=tls_config.go:350 msg="TLS is disabled." backend_type=tsdb http2=false address=127.0.0.1:9030
time=2025-02-13T16:43:51.776Z level=INFO source=helpers.go:55 msg="Starting health checker" backend_type=pyroscope
```

### Adding CEEMS LB and Pyroscope Datasources on Grafana

When CEEMS LB and Pyroscope have been deployed, in addition to the datasources configured for Grafana
in the [above section](#configuring-grafana), we need to add three new datasources: two for CEEMS LB (Prometheus
and Pyroscope backends) and one for vanilla Pyroscope (without any access control). A sample provisioned config
file for these datasources is shown below:

```yml
# Configuration file version
apiVersion: 1

# List of additional datasources that CEEMS uses
datasources:
  # Vanilla Pyroscope datasource that DOES NOT IMPOSE ANY ACCESS CONTROL
  - name: pyro
    type: pyroscope
    access: proxy
    # Replace it with Pyroscope URL
    url: <PYROSCOPE_URL>
    # If Pyroscope server has basic authentication
    # configured ensure that it has been added here as well

  - name: ceems-lb-tsdb
    # It should be of type Prometheus
    type: prometheus
    access: proxy
    url: http://localhost:9030
    basicAuth: true
    basicAuthUser: <CEEMS_LB_BASIC_AUTH_USERNAME>
    jsonData:
      prometheusVersion: 2.51
      prometheusType: Prometheus
      timeInterval: 30s
      incrementalQuerying: true
      cacheLevel: Medium
      # This is CRUCIAL. We need to send this header for CEEMS LB
      # to proxy the request to correct backend
      httpHeaderName1: X-Ceems-Cluster-Id
    secureJsonData:
      basicAuthPassword: <CEEMS_LB_BASIC_AUTH_PASSWORD>
      # It must be the same `id` configured across CEEMS components
      httpHeaderValue1: slurm-cluster

  - name: ceems-lb-pyro
    # It should be of Pyroscope type
    type: pyroscope
    access: proxy
    url: http://localhost:9040
    basicAuth: true
    basicAuthUser: <CEEMS_LB_BASIC_AUTH_USERNAME>
    jsonData:
      # This is CRUCIAL. We need to send this header for CEEMS LB
      # to proxy the request to correct backend
      httpHeaderName1: X-Ceems-Cluster-Id
    secureJsonData:
      basicAuthPassword: <CEEMS_LB_BASIC_AUTH_PASSWORD>
      # It must be the same `id` configured across CEEMS components
      httpHeaderValue1: slurm-cluster
```

After replacing the placeholders, this file must be installed at `/etc/grafana/provisioning/datasources`
folder and restart Grafana server.

Finally, while importing [dashboards](https://github.com/mahendrapaipuri/ceems/tree/main/thirdparty/grafana),
the datasources for [SLURM Single Job Metrics](https://github.com/mahendrapaipuri/ceems/blob/main/thirdparty/grafana/dashboards/slurm/slurm-single-job-metrics.json)
and for [Openstack Single VM Metrics](https://github.com/mahendrapaipuri/ceems/blob/main/thirdparty/grafana/dashboards/openstack/os-single-vm-metrics.json)
must be configured as `ceems-lb-tsdb` and `ceems-lb-pyro` (only for SLURM). This ensures that the queries
made by Grafana will be intercepted by CEEMS LB, enforce the access control and then decide whether to proxy
request to backend or not.

## Conclusion

This guide provides an overall view of all necessary steps needed to configure CEEMS, Prometheus
and Grafana. This should be only used as a guide and it must be adopted to the needs and constraints
of individual data center. Any suggestions to improve this guide are always welcome and please do not
hesitate to open a bug report, if any errors are found here.
