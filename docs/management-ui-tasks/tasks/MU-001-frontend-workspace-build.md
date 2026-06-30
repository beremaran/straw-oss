# MU-001: Frontend Workspace And Build Integration

Status: done
Phase: 1
Depends on: none
Search tags: frontend workspace, dev server, build, test, `web/management`, static assets

## Objective

Create the minimal frontend workspace needed to build and run the Management UI inside this repository.

## Scope

- Add a Management UI app workspace under `web/management` or another documented frontend path.
- Choose and document the package manager, dev server command, build command, and test command.
- Add only the dependencies needed for the first release UI; prefer the stack defaults before custom tooling.
- Configure static asset output so the UI can later be served or packaged without changing page code.
- Wire basic lint/type/test commands if the chosen stack supports them.

## Repo Touchpoints

- `web/management/*`
- root package/workspace files, if the chosen stack needs them
- `.gitignore`
- `Makefile`, only if useful for one-line local commands

## Implementation Tasks

- [x] Create the frontend project skeleton.
- [x] Add a runnable placeholder route that proves the app boots.
- [x] Document install, dev, build, and test commands.
- [x] Ensure generated/build output is ignored.
- [x] Keep the backend build and tests unaffected.

## Done Criteria

- [x] A developer can run the Management UI locally with one documented command after installing dependencies.
- [x] A production build command produces static assets.
- [x] The repository still passes existing backend checks.
- [x] No Management API behavior changes are required by this task.

