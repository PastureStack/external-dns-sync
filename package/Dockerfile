ARG UBUNTU_IMAGE=ubuntu:26.04@sha256:678c6550cc43645e08669028bc177f50be4e7c5b8cca677067b1914d4afc7a03
FROM ${UBUNTU_IMAGE}

# Modified by PastureStack contributors for independent maintenance and rebranding.

ARG VERSION
ARG REVISION
ENV DEBIAN_FRONTEND=noninteractive

COPY ubuntu-apt.lock /licenses/ubuntu-apt.lock
ADD --checksum=sha256:6077d27c6b6f8b23590cb01ff877ed8c804a67a5442cc32b5a33da10d2bd0e90 https://archive.ubuntu.com/ubuntu/pool/main/c/ca-certificates/ca-certificates_20260601~26.04.1_all.deb /tmp/ca-certificates.deb

RUN set -eux; \
    . /licenses/ubuntu-apt.lock; \
    rm -f /etc/apt/sources.list /etc/apt/sources.list.d/*.list /etc/apt/sources.list.d/*.sources; \
    rm -rf /tmp/ca-bootstrap; \
    mkdir -p /tmp/ca-bootstrap /etc/ssl/certs; \
    dpkg-deb --extract /tmp/ca-certificates.deb /tmp/ca-bootstrap; \
    find /tmp/ca-bootstrap/usr/share/ca-certificates -type f -name '*.crt' \
        | LC_ALL=C sort \
        | while IFS= read -r certificate; do sed -e '$a\' "${certificate}"; done \
        > /etc/ssl/certs/ca-certificates.crt; \
    test -s /etc/ssl/certs/ca-certificates.crt; \
    printf 'Types: deb\nURIs: https://snapshot.ubuntu.com/ubuntu/%s\nSuites: resolute resolute-updates resolute-backports resolute-security\nComponents: main universe restricted multiverse\nSigned-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg\nSnapshot: no\n' \
        "${UBUNTU_APT_SNAPSHOT}" > /etc/apt/sources.list.d/pasturestack-snapshot.sources; \
    printf 'Acquire::Retries "5";\nAcquire::http::Timeout "30";\nAcquire::https::Timeout "30";\nAcquire::https::CaInfo "/etc/ssl/certs/ca-certificates.crt";\nAcquire::https::Verify-Peer "true";\nAcquire::https::Verify-Host "true";\nAcquire::AllowInsecureRepositories "false";\nAPT::Get::AllowUnauthenticated "false";\n' > /etc/apt/apt.conf.d/80pasturestack-retries; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
        ca-certificates="${UBUNTU_APT_CA_CERTIFICATES_VERSION}"; \
    { \
        printf 'snapshot\t%s\n' "${UBUNTU_APT_SNAPSHOT}"; \
        dpkg-query -W -f='${binary:Package}\t${Version}\n' | LC_ALL=C sort; \
    } > /licenses/EXTERNAL-DNS-SYNC-RUNTIME-UBUNTU-PACKAGES.tsv; \
    apt-get clean; \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* /usr/bin/pebble; \
    groupadd --gid 10001 pasturestack && \
    useradd --uid 10001 --gid 10001 --create-home --home-dir /home/pasturestack --shell /usr/sbin/nologin pasturestack

COPY --chown=10001:10001 entrypoint.sh update-platform-ca external-dns-sync /usr/local/bin/
COPY LICENSE SECURITY.md ORIGIN.md COMPATIBILITY.md THIRD_PARTY_NOTICES.md /licenses/
COPY third-party-licenses/ /licenses/third-party/

RUN chmod 0755 /usr/local/bin/entrypoint.sh /usr/local/bin/update-platform-ca /usr/local/bin/external-dns-sync && \
    find /licenses -type f -exec chmod 0644 {} +

LABEL org.opencontainers.image.title="external-dns-sync" \
      org.opencontainers.image.description="External DNS synchronization service maintained by PastureStack" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.source="https://github.com/PastureStack/external-dns-sync" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"

ENV HOME=/home/pasturestack
EXPOSE 10000
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
