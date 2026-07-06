# Observability Assets

Grafana can provision the Straw dashboard from:

- `grafana/provisioning/dashboards/straw.yml`
- `grafana/dashboards/straw-operational-overview.json`

The dashboard expects a Prometheus data source named `Prometheus` by default. Override the `DS_PROMETHEUS`
dashboard variable when importing into an environment that uses a different data source name.

The panels intentionally use only bounded Prometheus labels from `docs/planning/23-observability.md`
(`tenant_id`, `route_id`, `target_host`, `error_code`) plus service scrape labels. Full URLs, worker IDs, and
tenant-private topology do not belong in shared dashboards.
