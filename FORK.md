# AnotherGenZ/dnsweaver: fork notes

This fork tracks [maxfield-allison/dnsweaver](https://github.com/maxfield-allison/dnsweaver) and carries a small
set of changes that upstream doesn't have. Everything else is upstream, unchanged.

## What the fork adds

| Change | Why | Where |
|---|---|---|
| `DNSWEAVER_{NAME}_MATCH_LABELED_ONLY` (opt-in instances) | Split horizon where every app is public but only some are internal: the internal instance must not receive unlabeled apps | `internal/config`, `pkg/provider`, `internal/reconciler/desired.go` (`selectInstances`), docs `configuration/multi-instance.md#opt-in-instances-fork` |
| `dnsweaver.provider` / `dnsweaver.dev/provider` as aliases of `dnsweaver.instances` | Existing deployments use the fork's 1.x label | `pkg/source/instances.go` |
| `fork-release` workflow | Upstream publishes from GitLab; the fork publishes `ghcr.io/anothergenz/dnsweaver` | `.github/workflows/fork-release.yml` |

Dropped in the 3.x sync (now upstream, or superseded): Technitium object `svcParams`, Cloudflare account-token
verification, and the configurable `DNSWEAVER_DETACHED_CLEANUP_*` breaker (upstream has a fixed mass-deletion
breaker, incomplete-discovery suppression and exact ownership). Remove those env vars from deployments.

## Syncing with upstream

Merge upstream releases; never rebase `main` (it's public).

```sh
git fetch upstream --tags
git switch -c sync/upstream-vX.Y.Z main
git merge vX.Y.Z            # conflicts, if any, are in the files listed above
go test -race ./... && golangci-lint run --config .golangci.yml
```

Open a PR to `main`. After it merges, tag `vX.Y.Z-fork.1` to publish the image.

Workflows disabled in this fork's GitHub settings (kept in the tree so syncs don't conflict): `sync-to-gitlab`,
`website`, `docs`.

## Releases

Tags `v<upstream>-fork.<n>` build `linux/amd64` + `linux/arm64` and push `ghcr.io/anothergenz/dnsweaver:<tag>`.
`:latest` is not moved; deployments pin `<tag>@sha256:<digest>`.
