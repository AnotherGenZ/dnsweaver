# nginx-proxy Labels

dnsweaver can extract hostnames from Docker containers that declare a
`VIRTUAL_HOST` in the
[jwilder/nginx-proxy](https://github.com/nginx-proxy/nginx-proxy)
convention. This lets you manage DNS for services already configured to
sit behind nginx-proxy without duplicating hostname config.

## Enabling nginx-proxy

Add `nginx-proxy` to `DNSWEAVER_SOURCES`:

```yaml
environment:
  - DNSWEAVER_SOURCES=nginx-proxy
```

You can combine it with other sources:

```yaml
- DNSWEAVER_SOURCES=traefik,nginx-proxy,dnsweaver
```

## Labels vs Environment Variables

!!! warning "Labels only — for now"
    Upstream nginx-proxy reads `VIRTUAL_HOST` from the container
    **environment**. dnsweaver currently consumes Docker **labels**
    only. Until env-var plumbing lands, you must declare `VIRTUAL_HOST`
    as a label (in addition to, or instead of, an env var) on any
    container you want dnsweaver to manage.

dnsweaver recognises two label keys:

| Label | Notes |
| :---- | :---- |
| `VIRTUAL_HOST` | Literal jwilder env-var name used as a Docker label key |
| `com.nginx-proxy.virtual_host` | Canonical reverse-DNS label form |

Either form works; use whichever fits your labelling conventions.

## Label Format

### Single hostname

```yaml
labels:
  VIRTUAL_HOST: app.example.com
```

### Multiple hostnames

Comma-separated — same syntax as upstream nginx-proxy:

```yaml
labels:
  VIRTUAL_HOST: app.example.com,www.example.com
```

Whitespace around the commas is tolerated.

### Canonical label form

```yaml
labels:
  com.nginx-proxy.virtual_host: app.example.com
```

If both labels are present, their hostnames are merged and deduplicated.

## Supported Record Types

`VIRTUAL_HOST` declares **hostnames only**. dnsweaver pairs each
hostname with the default record type for the matching provider
(typically `A`). For per-hostname record overrides, combine this source
with [native dnsweaver labels](native-labels.md).

## Caveats

- **No env-var extraction.** If you run upstream jwilder/nginx-proxy
  and rely on `-e VIRTUAL_HOST=…` without a matching label, dnsweaver
  cannot see the hostname.
- **No nginx.conf parsing.** Only Docker labels are read; static nginx
  configs are out of scope.
- **No VIRTUAL_PORT or VIRTUAL_PATH handling.** dnsweaver only cares
  about hostnames; upstream/path routing belongs to nginx-proxy itself.
