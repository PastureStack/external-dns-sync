# Third-party notices

The internal metadata client is derived from
`github.com/rancher-archives/go-rancher-metadata` at commit
`11a77c258e8c11c6b33f4d4a5de733d17ccf8daf`.

That code is distributed under the Apache License 2.0. An exact copy of the
license is stored at `third_party/metadata-client/LICENSE` with SHA-256:

`0d542e0c8804e39aa7f37eb00da5a762149dc682d7829451287e11b938e94594`

The neutral `internal/platformapi` HTTP client was newly implemented for this
repository. Its JSON event contract was checked against the historical
`github.com/rancher/go-rancher` API client at commit
`939fd85e3c7f06f29e985fd35cb62a281a32d3b0`; no generated client source is
vendored or claimed as PastureStack-authored work.
