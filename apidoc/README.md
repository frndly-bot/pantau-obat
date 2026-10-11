# apidoc — OpenCollection Spec (API Contract)

Status: **complete and passing.** 75 endpoint files across 12 folders; one
collection run produces **75/75 requests passing with 159/159 assertions**, and
is idempotent (a second run against the same database still passes).
API contract document: `docs/design-v2/api-v2.md`.

This folder is the **backend's readiness gate**. The backend is not considered
done until every endpoint in `docs/design-v2/api-v2.md` has a file here, with an
example request and example response that actually run.

## Why OpenCollection

[OpenCollection](https://spec.opencollection.com/) (spec v1.0.0) is an open
format for describing API collections: requests, auth, variables, and scripts.
Chosen because:

- **Text format in the repo.** It can be diffed, reviewed, and committed. Not a
  binary file or a tool-specific export.
- **Multi-file per folder.** One endpoint, one file. No single giant file that
  conflicts when several people add endpoints.
- **Runnable as a test.** The `runtime` section supports scripts and assertions,
  so this folder is not just documentation but also an end-to-end test
  mechanism.
- **No tool lock-in.** It can be opened in Bruno, or any other tool that supports
  this spec.

## Folder structure

```
apidoc/
  opencollection.yml              # collection metadata + variables
  environments/
    local.yml                    # local base_url
    staging.yml                  # staging base_url
  authentication/
    register.yml
    login.yml
    refresh.yml
    logout.yml
    me.yml
    change-password.yml
  user/
    get-profile.yml
    update-profile.yml
    delete-account.yml
    audit-log.yml
  medication/
    list.yml
    create.yml
    get.yml
    update.yml
    delete.yml
    adherence.yml
  schedule/
    list-times.yml
    create-time.yml
    update-time.yml
    delete-time.yml
  log/
    list-by-medication.yml
    list-all.yml
    create.yml
    from-reminder.yml
    update.yml
    snooze.yml
    today.yml
  stock/
    get.yml
    set.yml
    transactions.yml
    restock.yml
    adjust.yml
    low.yml
  catalog/
    search.yml
    get.yml
    create.yml
    update.yml
    delete.yml
    suggest.yml
  caregiver/
    create-invite.yml
    list-links.yml
    accept.yml
    reject.yml
    update-link.yml
    revoke.yml
    overview.yml
    medications.yml
    adherence.yml
    logs.yml
    create-log.yml
  notification/
    get-prefs.yml
    update-prefs.yml
    register-device.yml
    unregister-device.yml
    inbox.yml
  admin/
    list-users.yml
    get-user.yml
    disable-user.yml
    enable-user.yml
    erase-user.yml
    grant-admin.yml
    revoke-admin.yml
    list-catalog.yml
    create-catalog.yml
    update-catalog.yml
    publish-catalog.yml
    unpublish-catalog.yml
    archive-catalog.yml
    import-catalog.yml
    list-jobs.yml
    retry-job.yml
    cancel-job.yml
    stats.yml
  webhook/
    email-status.yml
    telegram-update.yml
  health/
    healthz.yml
    readyz.yml
```

Folders follow the **resource**, not the UI page. One endpoint = one file.

## File format

Follows the OpenCollection structure. Full example:

```yaml
info:
  name: Create Medication
  type: http
  seq: 2
  tags:
    - medication
    - smoke

http:
  method: POST
  url: "{{base_url}}/api/v1/medications"
  headers:
    - name: Content-Type
      value: application/json
  body:
    type: json
    data: |-
      {
        "name": "Amlodipine 10",
        "dose_value": 1,
        "dose_unit_code": "tablet",
        "route_code": "O",
        "meal_relation": "after_meal",
        "start_date": "2026-10-11",
        "frequency": { "pattern": "daily", "times_per_day": 2 },
        "times": [
          { "time_local": "07:00", "label": "morning" },
          { "time_local": "19:00", "label": "evening" }
        ]
      }
  auth:
    type: bearer
    token: "{{access_token}}"

runtime:
  assertions:
    - expression: res.status
      operator: eq
      value: "201"
    - expression: res.body.data.name
      operator: isString
  scripts:
    - type: after-response
      code: |-
        bru.setVar("medication_id", res.body.data.id);

docs: |-
  # Create Medication

  Creates a medication together with its frequency pattern, dose times, and initial stock.

  ## Notes
  - `frequency.pattern=daily` must include `times_per_day`.
  - `uq_meds_owner_name` rejects a duplicate name+dose+unit for the same user.
  - Returns 409 `DUPLICATE_MEDICATION` on a duplicate.

  Contract: `docs/design-v2/api-v2.md` section 5.
```

## Setting up the environment

Secret values (test admin password, webhook secrets) are **not** stored in files
that are committed to git. Copy the example and fill it in:

```bash
cp apidoc/.env.example apidoc/.env
# then edit apidoc/.env: ADMIN_PASSWORD, EMAIL_WEBHOOK_SECRET,
# TELEGRAM_WEBHOOK_SECRET — the values must match the backend you run
```

`environments/local.yml` reads those values via
`{{process.env.NAME}}` interpolation, so `local.yml` stays safe for a public
repo. `apidoc/.env` itself is ignored by the root `.gitignore`.

## Variables

Defined in `opencollection.yml`, per-environment values in
`environments/*.yml`. Secrets come from `apidoc/.env` (see above).

| Variable | Contents |
|---|---|
| `base_url` | `http://localhost:8080` (local) |
| `access_token` | set automatically by `authentication/login.yml` |
| `user_id` | set automatically by `authentication/register.yml` |
| `medication_id` | set automatically by `medication/create.yml` |
| `schedule_id` | set automatically by `schedule/create-time.yml` |
| `log_id` | set automatically by `log/create.yml` |
| `catalog_id` | set automatically by `catalog/create.yml` |
| `care_link_id` | set automatically by `caregiver/create-invite.yml` |
| `admin_token` | admin account token (from the seed, not register) |

This chain of variables is what lets the folder be used as a **sequential e2e
test**: register → login → create medication → log a dose → check stock → check
adherence.

## Run order (collection run)

The `seq` order is arranged so that a single run forms a real flow:

1. `health/healthz` — make sure the API is up
2. `authentication/register` — create a test account
3. `authentication/login` — get `access_token`
4. `catalog/search` — find a preset
5. `medication/create` — create medication + frequency + times + stock
6. `log/create` — log a dose, verify stock decreases
7. `stock/get` — confirm the balance dropped by one
8. `medication/adherence` — check the adherence figure
9. `caregiver/create-invite` — invite a caregiver
10. `admin/*` — with a separate `admin_token`

## Authoring rules

1. **One file, one endpoint.** Do not combine.
2. **Every file has `docs`.** Keep it concise: purpose, important notes, error
   codes that may occur, and a reference to `api-v2.md`.
3. **Every file has `runtime.assertions` with at least a status code.** For
   important endpoints, add assertions on meaningful fields.
4. **Example requests must actually run.** Not placeholders like
   `"name": "string"`. Use realistic values.
5. **Example responses are stored in `docs`.** Include one success example and one
   relevant failure example.
6. **Enums must use English values** that match the CHECK constraints in the
   migrations: `after_meal`, `morning`, `child`, `imported`. No Indonesian
   values.
7. **No secrets in files.** Use variables. Example passwords may be literal,
   but tokens may not. For real secrets (test admin password, webhook secrets),
   store them in `apidoc/.env` and reference them via `{{process.env.NAME}}`.
8. **Admin endpoints get their own folder.** Do not mix them with user
   endpoints, because `admin_scope` and auditing differ.

## Backend readiness

The backend is considered ready when **all** of these are met:

- [x] Every endpoint in `docs/design-v2/api-v2.md` has a file in `apidoc/`.
- [x] A collection run from scratch (empty database) passes without errors.
- [x] Every endpoint has at least one status-code assertion.
- [x] The e2e flow is meaningful: register → medication → log → stock drops → adherence rises.
- [x] The admin flow passes separately, including the `ADMIN_SCOPE_DENIED` test.
- [x] No `access_token` or secret is committed.
- [ ] `staging.yml` environment.
- [ ] `apidoc-run` target in CI with `set -o pipefail`.

A note on the last item: `make apidoc-run` uses `bru run`, and when its output
is piped (`| tail`), its exit status can be masked by the pipe. To use it as a CI
gate, that pipe needs `set -o pipefail` — otherwise a failing run passes green.

## What already exists

**75 endpoint files** across 12 folders, plus 12 `folder.yml`.

| Folder | Files | Contents |
|---|---|---|
| `health/` | 2 | `/healthz`, `/readyz` |
| `authentication/` | 9 | register, register-caregiver, login, login-admin, refresh, logout, me, change-password, relogin |
| `catalog/` | 6 | search, suggest, create, get, update, delete |
| `medication/` | 5 | create, list, get, update, delete |
| `log/` | 5 | create, from-reminder, update, snooze, today |
| `stock/` | 6 | get, set, restock, adjust, transactions, low |
| `caregiver/` | 11 | create-invite, list-links, accept, reject, update-link, revoke, overview, medications, adherence, logs, create-log |
| `adherence/` | 3 | summary, daily, by-medication |
| `notification/` | 6 | get-prefs, update-prefs, register-device, list-devices, unregister-device, inbox |
| `user/` | 4 | get-profile, update-profile, audit-log, delete-account |
| `admin/` | 16 | list-users, get-user, disable/enable/erase-user, grant/revoke-admin, list/create/publish/unpublish/archive-catalog, list/retry/cancel-job, stats |
| `webhook/` | 2 | email-status, telegram-update |

Latest run result: **75/75 requests passing, 159/159 assertions**, and
**idempotent** — run twice against the same database it still passes.

## Folder order MUST use `folder.yml`

Bruno sorts folders **alphabetically**, not by flow. Without `seq`, `adherence/`
and `admin/` run **before** `authentication/`, so `access_token` does not exist
yet and dozens of requests carry a cascade of 401s — it looks like a backend bug
when it is not.

Every folder therefore has a `folder.yml` containing `info.seq`:

| seq | Folder | seq | Folder |
|---|---|---|---|
| 1 | health | 7 | caregiver |
| 2 | authentication | 8 | adherence |
| 3 | catalog | 9 | notification |
| 4 | medication | 10 | user |
| 5 | log | 11 | admin |
| 6 | stock | 12 | webhook |

Adding a new folder **must** give it a `folder.yml` with a `seq`, inserted
without shifting the others when possible.

## What is missing

- `staging.yml` environment.
- `apidoc-run` target in CI.
- An `ADMIN_SCOPE_DENIED` test that actually hits a scope-limited admin. Today it
  is only tested indirectly.

## Three pitfalls already found (do not repeat)

1. **`bru.getVar` does not read environment variables.** It only reads variables
   set via `bru.setVar` (runtime). Variables from `environments/*.yml` must be
   read with **`bru.getEnvVar`**. Proven: the webhook secret read as empty via
   `getVar`, so the HMAC signature was always wrong and the endpoint returned
   401.

2. **`isObject` is not a known Bruno assertion operator.** The valid operators
   are: `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `in`, `notIn`, `contains`,
   `notContains`, `length`, `matches`, `notMatches`, `startsWith`, `endsWith`,
   `between`, `isEmpty`, `isNotEmpty`, `isNull`, `isUndefined`, `isDefined`,
   `isTruthy`, `isFalsy`, `isJson`, `isNumber`, `isString`, `isBoolean`,
   `isArray`. An unknown operator falls back to `eq` and compares against its own
   name — the assertion fails even when the response is correct.

3. **The webhook body must be type `text`, not `json`.** If it is `json`, Bruno
   parses then re-serializes the body, so the bytes sent no longer match the
   bytes signed. The server returns 401 even when the HMAC is correct.

## References

- Spec: <https://spec.opencollection.com/> (v1.0.0)
- YAML structure reference: <https://docs.usebruno.com/v3/opencollection-yaml/structure-reference>
- Endpoint contract: `docs/design-v2/api-v2.md`
- Backend rules: `backend/README.md`
