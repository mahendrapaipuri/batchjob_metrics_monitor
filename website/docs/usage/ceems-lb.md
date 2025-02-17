---
sidebar_position: 3
---

# CEEMS Load Balancer

## Basic usage

CEEMS load balancer can be started with its core and web configuration files as follows:

```bash
ceems_lb --config.file=/path/to/core/config/file --web.config.file=/path/to/web/config/file
```

This will start CEEMS load balancer at a default port `9030` listening on all interfaces.
To change default port and host, `--web.listen-address` CLI argument must be passed to
binary

```bash
ceems_lb --web.listen-address="localhost:8030"
```

:::tip[TIP]

All the available command line options are listed in
[CEEMS LB CLI docs](../cli/ceems-lb.md).

:::

CEEMS LB supports both TSDB and Pyroscope backends and it starts a load balancing server
for each of TSDB and Pyroscope when both backends are configured. In order to control the
address of each instance of CEEMS LB, `--web.listen-address` can be repeated. The first
configured address will be used for TSDB and second one for Pyroscope. For example, when
CEEMS LB is launched as follows:

```bash
ceems_lb --web.listen-address="localhost:7030" --web.listen-address=":10030"
```

the TSDB load balancer will be reachable at `localhost:7030` while the Pyroscope load
balancer will be available at `localhost:10030`.

## Access control

CEEMS load balancer is capable of providing basic access control for
TSDB using CEEMS API server. For this work, CEEMS load balancer configuration file must
include configuration related to CEEMS API server as discussed in [Configuration](../configuration/ceems-lb.md)
section. If CEEMS load balancer have access to the data directory of CEEMS API server,
load balancer will query the DB directly to enforce the access control.

If data directory of CEEMS API server is not accessible to CEEMS load balancer, it
is possible to configure the client configuration of CEEMS API server in CEEMS load
balancer and load balancer will make API requests to API server to know the ownership
details of a given compute unit before enforcing access control.

:::important[IMPORTANT]

As described in [CEEMS API Server](./ceems-api-server.md#access-control), Grafana must
be configured to send user header in the requests to datasource for access control to
work.

:::

## Using with Grafana

As discussed in [CEEMS Load Balancer](../components/ceems-lb.md) section, it is
possible for a single instance of CEEMS load balancer to support multiple clusters at
the same time. Let's take a sample CEEMS load balancer config file as follows:

```yaml
ceems_lb:
  strategy: round-robin
  backends:
    - id: slurm-one
      tsdb_urls:
        - http://slurm-one-tsdb-one:9090
        - http://slurm-one-tsdb-two:9090
    - id: slurm-two
      tsbd_urls:
        - http://slurm-two-tsdb-one:9090
        - http://slurm-two-tsdb-two:9090

ceems_api_server:
  data:
    path: /var/lib/ceems
```

It is clear from the config that there two different SLURM clusters, namely `slurm-one`
and `slurm-two`. Each cluster has its own dedicated set of TSDB instances.

Conventionally operators configure two different datasources on Grafana one for each
cluster. In the current case, the frontend load balancer of both clusters is CEEMS
load balancer and it is a single instance. Then the question pops up here: How do we
target correct cluster when configuring data source?

This is done using custom header, _i.e.,_ when configuring the data source for cluster
`slurm-one`, a custom header `X-Ceems-Cluster-Id` must be configured to `slurm-one`
and similar configuration must be done to `slurm-two`. CEEMS load balancer will look up
this custom header and load balance the traffic between the TSDB instances
of correct target cluster.

:::important[IMPORTANT]

Even there is only one cluster, it is necessary to add the custom header to the TSDB
Grafana datasource configuration.

:::

Thus the difference in configuring Prometheus datasource on Grafana compared to vanilla
Prometheus TSDB and CEEMS load balancer is the addition of this custom header in datasource
configuration.

## Admin users

CEEMS load balancer supports admin users with privileged access to TSDB. These users that
are configured as admin users in CEEMS API server will have admin privileges to CEEMS
load balancer as well. CEEMS load balancer will allow admin users to query data of
_any_ compute unit. It is not possible to have admin privileges on CEEMS load balancer
without any admin privileges on CEEMS API server.
