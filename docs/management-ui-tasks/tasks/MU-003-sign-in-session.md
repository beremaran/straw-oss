# MU-003: Sign-In, Token Handling, And Session Lifecycle

Status: not-started
Phase: 1
Depends on: MU-002
Search tags: `/login`, auth, local storage, remember connection, healthz, management token, sign out

## Objective

Implement connection sign-in, token safety, remembered connection behavior, and sign-out.

## Scope

- Build `/login` with Management API URL, management token, and Remember this connection toggle.
- Default the Management API URL to `http://localhost:8081`.
- Validate that the URL is present and includes a protocol.
- Validate that the token is present.
- Connect by checking `GET /healthz`, then an authenticated lightweight request such as `GET /management/api-keys?limit=1`.
- Persist base URL and token only when remember is enabled, with the local-storage warning from the spec.
- Keep unremembered credentials in memory for the tab session.
- Clear stored token and query cache on sign-out.

## Repo Touchpoints

- `web/management/src/routes/login*`
- `web/management/src/auth/*`
- `web/management/src/api/*`
- browser storage helpers

## Implementation Tasks

- [ ] Create sign-in form, validation, submit state, and error display.
- [ ] Handle `401` as "Invalid management token".
- [ ] Show network/CORS failures with attempted URL and connection hints.
- [ ] Never log or display the management token after entry.
- [ ] Redirect authenticated users to `/overview` and unauthenticated deep links to `/login`.

## Done Criteria

- [ ] Valid connection details reach `/overview`.
- [ ] Invalid token stays on `/login` and does not persist the token.
- [ ] Sign-out removes persisted credentials and cached management data.
- [ ] Raw API keys and management tokens are never persisted by accident.

