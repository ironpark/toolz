---
depends_on:
- "00-internal-codex#2"
perf_phase: false
status: planned
---
> DONE-WHEN: All auth methods have wire-shape + decode tests; AwaitLogin ignores
> NEXT: none

# Auth surface

## Planned Work

- `auth.go`: `ReadAccount(ctx, refreshToken bool)` returning typed account
  info (apiKey / chatgpt / chatgptAuthTokens variants with plan type and
  `requiresOpenaiAuth`); `LoginAPIKey(ctx, key)`; `LoginChatGPT(ctx)`
  returning `loginId` + `authUrl`; `LoginChatGPTDeviceCode(ctx)` returning
  verification URL + user code; `CancelLogin(ctx, loginID)`; `Logout(ctx)`.
- Login completion: expose `account/login/completed` and `account/updated`
  notifications via a channel or callback so callers can await a specific
  `loginId`; helper `AwaitLogin(ctx, loginID)` built on it.
- `auth_test.go`: fake-server tests for account/read variants (nil account,
  apiKey, chatgpt), API-key login happy path including both notifications,
  browser-flow start + cancel, and AwaitLogin resolving on the matching
  loginId only.

## Done When

- All auth methods have wire-shape + decode tests; AwaitLogin ignores
  non-matching loginIds and honors context cancellation; documented example
  payloads for account/read all decode.
