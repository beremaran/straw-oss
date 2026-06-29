# MU-012: Usage Summary, Billing Estimate, Filters, Charts, And CSV Export

Status: not-started
Phase: 3
Depends on: MU-005, MU-007
Search tags: `/usage`, usage summary, billing estimate, date range, api_key_id, CSV, charts, cost units

## Objective

Implement usage and billing views from the existing daily summary and estimate endpoints.

## Scope

- Load usage from `GET /management/usage/summary?start=&end=&api_key_id=`.
- Load billing from `GET /management/billing/estimate?start=&end=&api_key_id=`.
- Add date presets for last 7 days, last 30 days, month to date, and custom.
- Add API key filter from `GET /management/api-keys`.
- Validate dates as `YYYY-MM-DD` and start on or before end.
- Show totals for requests, bytes, cost units, estimated USD, currency, and date range.
- Add daily requests visual, endpoint-tier breakdown visual, and data table alternative.
- Export loaded usage rows to client-side CSV with date range and API key ID in the filename.

## Repo Touchpoints

- `web/management/src/routes/usage*`
- `web/management/src/components/usage/*`
- `web/management/src/api/usage*`
- `web/management/src/api/billing*`

## Implementation Tasks

- [ ] Build filters and keep invalid date edits local until fixed.
- [ ] Format bytes as B, KB, MB, or GB.
- [ ] Label billing as an estimate, not an invoice.
- [ ] Show backend `400` date parse errors at the invalid field.
- [ ] Add empty state copy for new installs, no traffic, or missing summary job data.

## Done Criteria

- [ ] Usage and billing filters send `YYYY-MM-DD` dates and optional `api_key_id`.
- [ ] Daily table and chart alternatives expose the same data.
- [ ] CSV export includes only loaded data and a useful filename.
- [ ] Hourly drilldown, cost multiplier management, invoices, payments, and organizations are not exposed.

