# MU-011: Fingerprint Preset List, JSON Editor, Upsert, Duplicate, And Broadcast

Status: done
Phase: 2
Depends on: MU-005
Search tags: `/fingerprints`, fingerprint preset, JSON config, upsert, duplicate, broadcast, NATS, delete gap

## Objective

Implement fingerprint preset management using the current list, upsert, and broadcast APIs.

## Scope

- List presets from `GET /management/fingerprints`.
- Show ID, name, inferred browser family, user agent, updated date, used-by-rules count, and actions.
- Create and edit presets through `POST /management/fingerprints`.
- Lock ID during edit because posting the same ID updates.
- Duplicate a preset into a new ID.
- Edit arbitrary JSON object config, with optional helper fields for common keys.
- Copy config JSON.
- Broadcast all presets through `POST /management/fingerprints/broadcast` with confirmation.
- Show "Broadcast requested"; do not claim worker acknowledgment counts.

## Repo Touchpoints

- `web/management/src/routes/fingerprints*`
- `web/management/src/components/fingerprints/*`
- `web/management/src/api/fingerprints*`

## Implementation Tasks

- [x] Build preset table and mobile card equivalent.
- [x] Calculate used-by-rules count from routing rules.
- [x] Add create/edit dialog or route with JSON object validation.
- [x] Add duplicate flow that requires a new ID.
- [x] Add broadcast confirmation and broker-error display.

## Done Criteria

- [x] Fingerprint list, create, edit/upsert, duplicate, copy JSON, and broadcast work through existing APIs.
- [x] Arbitrary config object shapes are preserved.
- [x] Broadcast errors keep existing presets untouched in the UI.
- [x] Fingerprint delete is not shown until a Management API route exists.

