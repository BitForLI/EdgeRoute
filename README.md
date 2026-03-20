# EdgeCDN-X CoreDNS Plugin

`edgecdnx` is a CoreDNS plugin that routes DNS queries to EdgeCDN-X locations using Kubernetes CRDs and request metadata.

It supports:
- Dynamic service-based routing for `A` and `AAAA` queries
- Configurable dynamic answers as `A`/`AAAA` or `CNAME`
- Alternate response mode for gRPC-originated requests detected from incoming context metadata
- Direct node resolution for hostnames in the form `node.location.node.service`
- Prefix-list routing (IP/CIDR to location)
- Geo metadata lookup fallback when no prefix match is found
- Hash-based node selection with health-aware filtering
- Fallback locations when a primary location has no healthy node
- Authoritative zone responses for configured Zone CRDs (SOA/NS and related behavior)

## How It Works

For each DNS query:

1. If query type is `A` or `AAAA`:
- First check for a direct node request matching `nodename.location.node.service.`.
  - Validate that the referenced `Service` exists.
  - Load the referenced `Location`.
  - Find the named node inside the location's node group for the service cache.
  - Return an authoritative `A` or `AAAA` answer pointing at that node.
- Try to map `qname` to a `Service` CRD (`spec.domain` or `spec.hostAliases[].name`).
- Determine a location:
  - First from prefix routing (`PrefixList` CRDs using source IP or EDNS client subnet).
  - If prefix is missing or cache type is not available there, use geo lookup.
- Select a node in that location:
  - Filter by cache node group.
  - Skip nodes in maintenance mode.
  - Use deterministic hash on query name.
  - Enforce IPv4/IPv6 health condition based on query type.
- If no healthy node in the chosen location, iterate configured fallback locations.
  - Choose response mode:
    - Use `DNSResponseType` by default.
    - If gRPC incoming metadata is present in request context, use `GRPCResponseType` instead.
  - Return either:
    - `A` or `AAAA` with the selected node IP when response type is `A_AAAA`
    - `CNAME` to `node_name.location.node.original-request.` when response type is `CNAME`

2. Otherwise (or if no matching Service):
- Fall back to zone-authoritative behavior backed by `Zone` CRDs.
- Return:
  - `NXDOMAIN` (+ SOA in authority section) if name does not exist
  - `NODATA` style response (empty answer + SOA in authority) if name exists but no RR of requested type
  - Normal answer when matching records exist

3. If this plugin cannot answer, request is passed to the next plugin in chain.

## Dependencies and Inputs

This plugin watches EdgeCDN-X CRDs via Kubernetes dynamic informers:
- `services`
- `locations`
- `prefixlists`
- `zones`

A working Kubernetes client configuration is required (`controller-runtime` `GetConfigOrDie()`), typically from in-cluster config or kubeconfig in the environment.

## Corefile Configuration

Syntax:

```txt
edgecdnx [ZONES...] {
  namespace <k8s-namespace>
  soa <primary-nameserver-label>
  ns <ns-hostname> <ipv4>
  ns <ns-hostname> <ipv4>
  recordttl <seconds>
  dnsresponsetype <CNAME|A_AAAA>
  grpcresponsetype <CNAME|A_AAAA>
}
```

Directives:

| Directive | Required | Default | Description |
| --- | --- | --- | --- |
| `namespace` | Yes | none | Kubernetes namespace to watch for EdgeCDN-X CRDs. |
| `soa` | Yes | none | SOA MNAME label prefix used when crafting SOA records (`<soa>.<zone>`). |
| `ns` | Recommended (repeatable) | empty | Adds NS and NS A records for each served zone. Format: `ns <hostname> <ipv4>`. |
| `recordttl` | No | `60` | TTL (seconds) used for generated `A`/`AAAA` node answers. |
| `dnsresponsetype` | No | `A_AAAA` | Allowed values: `CNAME`, `A_AAAA`. Used for normal DNS-originated dynamic responses. |
| `grpcresponsetype` | No | `CNAME` | Allowed values: `CNAME`, `A_AAAA`. Parsed and stored in plugin state. |

Notes:
- `dnsresponsetype` and `grpcresponsetype` are validated and set in plugin configuration.
- Normal dynamic service routing uses `dnsresponsetype`.
- If gRPC metadata is present in the incoming request context, dynamic service routing uses `grpcresponsetype` instead.
- `CNAME` responses use the target format `node_name.location.node.original-request.`.
- Direct node requests matching `nodename.location.node.service.` always return `A` or `AAAA` from the resolved node IP.
- Values are case-insensitive in Corefile input (converted to uppercase before validation).

Example:

```txt
.:53 {
  errors
  health
  ready

  edgecdnx . {
    namespace edgecdnx
    soa ns1
    ns ns1.edge.example.com. 203.0.113.10
    ns ns2.edge.example.com. 203.0.113.11
    recordttl 60
    dnsresponsetype A_AAAA
    grpcresponsetype CNAME
  }

  prometheus :9153
  forward . 1.1.1.1 8.8.8.8
  cache 30
  reload
}
```

## Build and Patch Workflow

This repository builds a patched CoreDNS that includes `edgecdnx` in the directive list and plugin registry.

### Local Build

```bash
make build
```

What this does:
- Downloads CoreDNS source tarball for configured version
- Extracts source
- Applies patch from `patches/<version>/coredns.patch`
- Updates CoreDNS version string with `-edgecdnx-<gitsha|dev>` suffix
- Builds CoreDNS binary in `coredns-<version>/coredns`

Useful targets:

```bash
make download
make extract
make patch
make clean
```

### Container Image

The provided Dockerfile expects a built `coredns` binary in repository root:

```bash
cp coredns-1.14.1/coredns ./coredns
docker build -t edgecdnx-coredns:local .
```

Runtime details:
- Runs as non-root (distroless base)
- Grants `cap_net_bind_service` to bind DNS port 53
- Exposes `53/tcp` and `53/udp`

## Readiness and Operational Notes

- Plugin readiness is tied to informer sync for Zone, Service, and PrefixList watchers.
- If informers have not synced yet, CoreDNS `ready` integration will report not ready for this plugin.
- Logging uses CoreDNS plugin logger under `edgecdnx*` prefixes.

## Error Behavior

- Invalid Corefile directive arguments return plugin startup errors.
- Invalid `dnsresponsetype`/`grpcresponsetype` values are rejected at startup.
- Direct node requests fall through to the next plugin if the referenced service, location, or node cannot be resolved.
- On routing failures (service not found, geolookup failure, no healthy nodes), plugin falls through to next handler where applicable.

## Metrics

A Prometheus counter vector is defined:
- `coredns_edgecdnx_request_count_total{server="..."}`

At present, the counter is declared but not incremented in request handling code.

## Troubleshooting

### Plugin does not appear in CoreDNS

- Ensure patch was applied (`patches/1.14.1/coredns.patch`).
- Confirm rebuilt binary is used at runtime.
- Verify `edgecdnx` appears in patched CoreDNS plugin list.

### Startup panic or config errors

- Validate required directives are set with arguments:
  - `namespace`
  - `soa`
- Check each `ns` line has exactly 2 arguments.
- Check `recordttl` is an integer.
- Check response type values are one of `CNAME`, `A_AAAA`.

### Unexpected fallback to next plugin

- Verify Service CRD domain/host alias exactly matches queried FQDN.
- Verify Location has node group matching Service cache.
- Verify health conditions for selected `A`/`AAAA` path.
- Verify PrefixList destination location exists.

### Direct node hostname does not resolve

- Query name must match `nodename.location.node.service.` exactly.
- The `service` suffix must resolve to an existing Service CRD domain or host alias.
- The referenced location must exist.
- The referenced node must exist inside the location node group matching the service cache.

### Unexpected CNAME instead of A or AAAA

- Check `dnsresponsetype` in Corefile for normal DNS-originated requests.
- Check `grpcresponsetype` for requests that carry gRPC incoming metadata in context.

## Development

Minimum toolchain:
- Go `1.25.x`
- `make`
- `patch`
- `curl`
- Docker (optional, for image build)

Basic checks:

```bash
go test ./...
```

## License

See project-level licensing in this repository or organization policy.
