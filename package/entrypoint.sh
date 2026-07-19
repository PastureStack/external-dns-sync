#!/bin/bash
# Modified by PastureStack contributors for independent maintenance and rebranding.
set -euo pipefail

# A compatible control plane injects these literal historical protocol
# variables for agent-role services. Prefer neutral settings when supplied.
export PLATFORM_URL="${PLATFORM_URL:-${CATTLE_URL:-}}"
export PLATFORM_ACCESS_KEY="${PLATFORM_ACCESS_KEY:-${CATTLE_ACCESS_KEY:-}}"
export PLATFORM_SECRET_KEY="${PLATFORM_SECRET_KEY:-${CATTLE_SECRET_KEY:-}}"

# The first argument is a service flag.
if [ "${1:-}" != "" ] && [ "${1:0:1}" = '-' ]; then
	set -- /usr/local/bin/external-dns-sync "$@"
fi

# No explicit command was supplied.
if [ "${1:-}" = "" ]; then
	set -- /usr/local/bin/external-dns-sync
fi

ca_bundle=$(/usr/local/bin/update-platform-ca)
export SSL_CERT_FILE="${ca_bundle}"

exec "$@"
