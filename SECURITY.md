<!-- Modified by PastureStack contributors for independent maintenance and rebranding. -->

# Security Notes

External DNS Sync is a privileged compatibility service. It can read environment
metadata, call the environment API, and create, update, or delete DNS records.
Do not expose it directly to untrusted users or public networks.

## Credential handling

- Grant provider credentials access only to the DNS zones and operations the
  service requires.
- Treat `PLATFORM_ACCESS_KEY`, `PLATFORM_SECRET_KEY`, provider credentials, TSIG
  secrets, and mounted secret files as sensitive data.
- Platform API requests are restricted to the exact scheme, hostname, and
  effective port approved during client initialization. Redirects and ambient
  HTTP proxies cannot move scoped control-plane credentials to another origin.
- Historical Infoblox source accepts secret files only from `/run/secrets` and
  bounds their size. Paths and symbolic links cannot escape that root.
- Do not enable debug or file logging unless logs are protected as operational
  secrets.
- Do not place provider credentials, private metadata, or private API endpoints
  in issues or source files.

## Runtime identity and CA boundary

The runtime helper imports a platform-managed CA certificate from
`PLATFORM_CA_ROOT`, which defaults to
`/var/lib/pasturestack/etc/ssl/ca.crt`. It creates a private combined CA bundle
under `/tmp` and points the process at that file; it does not modify the system
trust store. The runtime executes as non-root UID/GID `10001:10001`.
Deployments must mount only the required certificate path read-only.

## DNS update boundary

- Unit tests must use stub providers and must not contact real DNS zones.
- Provider integration tests require a dedicated disposable zone and separate
  least-privilege credentials.
- State TXT records identify records managed by this service. Changing their
  naming or ownership semantics requires a migration test that proves unrelated
  records cannot be deleted.
- Route 53 changes use a service-specific PastureStack comment; this comment is
  descriptive and is not an authorization control.

## Build boundary

`Dockerfile.dapper` is a build-only image. It mounts the source tree and Docker
socket to build and package the runtime image. It must not be deployed as a
production service.

Build and runtime images use a digest-pinned Ubuntu 26.04 base and the
`20260808T000000Z` Ubuntu snapshot with exact direct-package versions. The
build image uses Go 1.26.5 and Docker CLI 29.7.2 from SHA-256-verified official
archives.
Vendored source is treated as attacker-readable. Disabled legacy build paths
must fail closed and must not be restored without review.

## Compatibility boundary

The metadata hostname, `io.pasturestack.*` labels, `PLATFORM_*` variables, CA
input path, and event resource endpoint are documented in
[COMPATIBILITY.md](COMPATIBILITY.md). Any later change requires coordinated
contract tests.

## Reporting

Report vulnerabilities privately to the PastureStack maintainers. Include a
minimal reproduction with all credentials, account data, private domains, and
private endpoints removed.
