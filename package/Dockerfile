ARG ALPINE_IMAGE=alpine:3.23@sha256:1beb0dc0a51de7ff38e3b5274078a2e0b81113ba5c7535e1a03d5913a5edbda3
FROM ${ALPINE_IMAGE}

# Modified by PastureStack contributors for independent maintenance and rebranding.

ARG VERSION
ARG REVISION
COPY ubuntu-apt.lock /licenses/ubuntu-build-apt.lock

RUN apk add --no-cache \
        bash=5.3.3-r1 \
        ca-certificates=20260611-r0 && \
    apk info -v | LC_ALL=C sort > /licenses/EXTERNAL-DNS-SYNC-RUNTIME-ALPINE-PACKAGES.tsv && \
    addgroup -S -g 10001 pasturestack && \
    adduser -S -D -h /home/pasturestack -s /sbin/nologin -G pasturestack -u 10001 pasturestack

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
