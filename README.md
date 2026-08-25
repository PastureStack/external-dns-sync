<!-- Modified by PastureStack contributors for independent maintenance and rebranding. -->

# External DNS Sync

> PastureStack is an independent community effort to preserve, audit, and modernize the Rancher 1.6 ecosystem. It is not affiliated with or endorsed by Rancher Labs or SUSE.

**Upstream:** [`rancher-archives/external-dns`](https://github.com/rancher-archives/external-dns). This GitHub fork retains the upstream Git history, authorship, dates, and license notices unchanged; PastureStack maintenance is consolidated into one commit after the preserved upstream boundary.

External DNS Sync publishes service addresses from a compatible legacy
environment to an external DNS provider. It reads workload and host data from
the environment metadata service, calculates the desired DNS records, applies
provider changes, and reports service FQDNs through the environment API.

This repository preserves the complete upstream Git history and contributor
record. PastureStack contributors claim authorship only for their own changes.
See [ORIGIN.md](ORIGIN.md) for provenance and [COMPATIBILITY.md](COMPATIBILITY.md)
for the neutral runtime contract.

## Published provider

The reviewed `v0.8.1` runtime registers only Amazon Route 53 because that is
the first-party infrastructure template being preserved in this release.
Historical implementations for other providers remain in the upstream-derived
source tree for provenance and license continuity, but they are not registered
in or supported by the published image.

Provider credentials and permissions must be limited to the hosted zone managed
by this service.

## Record model

The default A-record name is:

```text
<service_name>.<stack_name>.<environment_name>.<root_domain>
```

For example, service `api` in stack `production`, environment `primary`, and
root domain `example.com` produces `api.production.primary.example.com`.

The service also maintains a TXT state record so it can distinguish records it
owns from unrelated records in the provider zone.

## Common configuration

| Setting | Required | Purpose |
| --- | --- | --- |
| `PLATFORM_URL` | Yes | Compatible environment API endpoint. |
| `PLATFORM_ACCESS_KEY` | Yes | Environment API access key. |
| `PLATFORM_SECRET_KEY` | Yes | Environment API secret key. |
| `ROOT_DOMAIN` | Yes | DNS zone suffix for generated records. |
| `NAME_TEMPLATE` | No | Record-name template; defaults to service, stack, and environment names. |
| `TTL` | No | DNS TTL in seconds; defaults to `300`. |
| `-provider` | No | Provider registry name; defaults to `route53`. |
| `-debug` | No | Enables debug logging. |
| `-log` | No | Writes logs to the specified file. |

## Route 53 configuration

| Setting | Required | Purpose |
| --- | --- | --- |
| `AWS_ACCESS_KEY_ID` | No | AWS access key; optional when an EC2 IAM role supplies credentials. |
| `AWS_SECRET_ACCESS_KEY` | No | Secret paired with the access key. |
| `AWS_SESSION_TOKEN` | No | Optional token for temporary credentials. |
| `AWS_REGION` | No | Signing region; defaults through the AWS SDK. |
| `ROUTE53_ZONE_ID` | No | Explicit hosted-zone ID when names are ambiguous. |
| `ROUTE53_MAX_RETRIES` | No | SDK retry limit from 0 through 10; defaults to 3. |
| `ROUTE53_ENDPOINT_URL` | No | Alternate HTTP(S) Route 53-compatible endpoint for isolated validation. |

Never place credentials in source files. The endpoint override is intended for
isolated testing or a deliberately reviewed compatible service; production AWS
deployments should leave it blank.

The metadata hostname, PastureStack labels, environment variables, and API
event shape are listed precisely in [COMPATIBILITY.md](COMPATIBILITY.md).
When launched by the reviewed Catalog template, the entrypoint maps the
compatible control plane's literal historical `CATTLE_*` agent variables to
the neutral `PLATFORM_*` contract. The runtime also reads the four documented
legacy DNS policy labels during an in-place upgrade, while neutral labels take
precedence for new deployments.

## Local build and test

The project uses a containerized Go 1.27.0 module build with a committed
`vendor/modules.txt` dependency lock. Route 53 uses AWS SDK for Go v2
`v1.65.9`; AliDNS uses Alibaba Cloud's maintained V2.0 SDK `v5.6.0`.
The end-of-life AWS SDK for Go v1, Aliyungo, and the legacy `vendor.conf`
workflow are not used.

```bash
VERSION_OVERRIDE=poc ARCH=amd64 make test
VERSION_OVERRIDE=poc ARCH=amd64 make validate
VERSION_OVERRIDE=poc ARCH=amd64 make build
```

The unit tests do not require a real DNS zone or provider credentials. Do not
run a provider update against a production zone as part of a source migration
proof of concept. CI/CD publication and release automation are outside this
scope.

After packaging a local image, the entrypoint and binary can be checked without
contacting a provider:

```bash
docker run --rm ghcr.io/pasturestack/external-dns-sync:v0.8.1 --help
```

## Runtime boundary

The process requires access to compatible metadata and environment APIs. It
also imports a platform-managed CA certificate from the configured PastureStack
path when present. The runtime executes as non-root UID/GID `10001:10001`; it
does not require privileged mode, host networking, a Docker socket, or write
access to the host filesystem. It is still a security-sensitive operational
component and must not be exposed as a public general-purpose service.

The dependency-aware health endpoint listens on the container network at port
`10000` by default. `GET /ping` returns `pong` only after metadata, Route 53,
and the environment API all respond successfully.

This component has no user interface or runtime localization subsystem.
Operational flags and logs remain English; no unsupported locale files are
included.

## Security and support

Read [SECURITY.md](SECURITY.md) before deployment. Use the PastureStack
repository issue tracker for project-specific reports, but never include DNS
credentials, private API endpoints, account identifiers, or unredacted logs.

The affiliation disclaimer at the top of this README applies to all builds and
distributions of this project.

## License

The project remains licensed under the existing [Apache License 2.0](LICENSE).
PastureStack has not replaced the license. Upstream copyright notices and the
license and notice files of vendored dependencies must be preserved. The local
packaging step copies discovered vendored legal files into
`/licenses/third-party/` in the runtime image.
