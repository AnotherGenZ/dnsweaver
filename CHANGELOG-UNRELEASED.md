## Unreleased

- Feature: Auto-create companion HTTPS (SVCB) records for Technitium provider (issue #158)
  - Enabled by default — prevents ECH fallback errors in split-horizon DNS
  - Configurable via `AUTO_HTTPS_RECORDS` (default: `true`) and `AUTO_HTTPS_ALPN` (default: `h2`)
  - Safe: skips creation if HTTPS record already exists; lifecycle-managed with parent record
