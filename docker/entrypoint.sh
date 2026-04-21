#!/bin/sh
# =============================================================================
# dnsweaver - Container entrypoint
# =============================================================================
#
# Handles Docker socket GID auto-detection so the standard compose example
# works out of the box. Without this, users have to look up their host's
# docker group GID and add it via `group_add` in compose, which is friction
# we don't need.
#
# Behavior:
#   - If /var/run/docker.sock is mounted, read its GID and add the dnsweaver
#     user to a group with that GID (creating the group if needed).
#   - If no socket is mounted (k8s-only mode, socket proxy, etc.), skip the
#     logic entirely and just drop privileges.
#   - Always exec the binary as the unprivileged dnsweaver user via su-exec.
#
# Manual escape hatch: users can still set `group_add` in compose for unusual
# cases (rootless Docker with no group on the socket, etc.).
# =============================================================================

set -e

DOCKER_SOCK="${DOCKER_SOCK:-/var/run/docker.sock}"

if [ -S "$DOCKER_SOCK" ]; then
    SOCK_GID=$(stat -c '%g' "$DOCKER_SOCK")

    # Skip if dnsweaver already has access (socket world-readable, GID matches
    # dnsweaver's primary group, or running on a system where socket has no
    # restrictive group).
    if [ "$SOCK_GID" != "0" ] && ! id -G dnsweaver | tr ' ' '\n' | grep -qx "$SOCK_GID"; then
        # Find or create a group with the socket's GID
        GROUP_NAME=$(getent group "$SOCK_GID" | cut -d: -f1)
        if [ -z "$GROUP_NAME" ]; then
            GROUP_NAME="docker-host"
            addgroup -g "$SOCK_GID" "$GROUP_NAME" 2>/dev/null || true
        fi
        # Add dnsweaver to the group (idempotent; ignore failure)
        addgroup dnsweaver "$GROUP_NAME" 2>/dev/null || true
    fi
fi

# Note: use `dnsweaver` (not `dnsweaver:dnsweaver`) so su-exec picks up
# supplementary groups from /etc/group. Specifying an explicit group resets
# supplementary groups to the empty set, which would undo the docker-host
# membership we just added.
exec su-exec dnsweaver /usr/local/bin/dnsweaver "$@"
