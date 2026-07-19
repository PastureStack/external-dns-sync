<!-- Modified by PastureStack contributors for independent maintenance and rebranding. -->

# Compatibility contracts

External DNS Sync uses PastureStack names for the repository, executable,
container image, entrypoint, internal application types, and new project-facing
text. The POC exposes the following neutral identifiers.

## Environment API

- `PLATFORM_URL`
- `PLATFORM_ACCESS_KEY`
- `PLATFORM_SECRET_KEY`
- Event type `dns.update`
- Event resource endpoint `externalDnsEvents`

The minimal HTTP client is implemented in `internal/platformapi`; an
upstream-branded client package is not part of the public dependency surface.
The entrypoint accepts the literal historical `CATTLE_URL`,
`CATTLE_ACCESS_KEY`, and `CATTLE_SECRET_KEY` values injected by a compatible
agent-role scheduler and maps them to `PLATFORM_*`. Those names are protocol
identifiers, not product branding.

## Metadata service

- URL `http://metadata/2015-12-19`; isolated validation may override it with
  `METADATA_URL`
- Service label `io.pasturestack.service.external_dns`
- Service template label `io.pasturestack.service.external_dns_name_template`
- Host policy label `io.pasturestack.host.external_dns`
- Host address label `io.pasturestack.host.external_dns_ip`

The neutral names take precedence. For in-place upgrades, the runtime also
reads the literal historical `io.rancher.service.external_dns`,
`io.rancher.service.external_dns_name_template`,
`io.rancher.host.external_dns`, and `io.rancher.host.external_dns_ip`
protocol labels. They are bounded compatibility identifiers, not product
branding. New deployments should use the neutral names.

## Runtime filesystem

The CA input path defaults to `/var/lib/pasturestack/etc/ssl/ca.crt` and can be
overridden with `PLATFORM_CA_ROOT`. The non-root entrypoint combines it with
the Ubuntu CA bundle under `/tmp` and exports `SSL_CERT_FILE`.

The health endpoint listens on `:10000` by default and can be overridden with
`HEALTH_ADDRESS`. `GET /ping` returns `pong`; `HEAD /ping` returns an empty
successful response after checking metadata, the selected provider, and the
platform API.

## DNS ownership state

The `external-dns-<environment UUID>.<root domain>` TXT record prefix is
preserved. It identifies A records managed by this service and prevents blind
deletion of unrelated provider records.

## Provider boundary

The reviewed `v0.8.0` image registers only the `route53` provider. Other
provider implementations inherited in the source tree are not included in the
published runtime registration surface.

## New artifact names

- Repository and source import: `github.com/PastureStack/external-dns-sync`
- Executable: `external-dns-sync`
- Container image: `ghcr.io/pasturestack/external-dns-sync`
- Runtime entrypoint: `/usr/bin/entrypoint.sh`

This component has no browser UI or runtime localization framework. Operational
flags and logs remain English, and no unsupported locale files are included.
