# SP Chat provisioning (New API ↔ SP Chat copy)

## Model
1 New API end-user = 1 company = 1 SP Chat org.

## Scope
- **Live New API**: `/root/new-api` (docker `new-api` on `127.0.0.1:3000`, public via `https://api.spit.hk`)
- **SP Chat COPY only**: `/root/open-webui-mvp-copy-20261029T124741+0800` on `127.0.0.1:8089`
- Live SP Chat (`/root/open-webui-mvp`, `:8088`/`:8080`) is **not** modified.

## Env (docker `--env-file`)
| Variable | Purpose |
|---|---|
| `SP_CHAT_PROVISION_ENABLED` | `true`/`false` (default true) |
| `SP_CHAT_BASE_URL` | From container use `http://172.17.0.1:8089` (socat → host `127.0.0.1:8089`) |
| `SP_CHAT_ADMIN_TOKEN` | JWT for COPY platform admin (auto-refreshed if email/password set) |
| `SP_CHAT_ADMIN_EMAIL` | `sp-chat-provision@spit.hk` (COPY-only service admin) |
| `SP_CHAT_ADMIN_PASSWORD` | Password for token refresh |

File on host: `/root/new-api/data/sp-chat-provision.env` (merged into container env at deploy).

## Networking
`socat-owui-copy-8089.service` forwards `172.17.0.1:8089` → `127.0.0.1:8089` so the New API container can reach the COPY without binding OWUI publicly.

## Trigger
Provisioning (`service.ProvisionSpChatOrg`) needs a non-empty `company_name`, an email that is verified or admin-asserted, and the plain password:
- **Admin CreateUser** (primary path): admin supplies email + company name.
- **Register**: only when `EmailVerificationEnabled` (email proven by code). With verification off, the company name is stored, no email is stored, and nothing is provisioned — so an unverified address can never claim an SP Chat login.
- **Reseller Create Customer** (B2B2C tree `/root/new-api3`, primary path there): `POST /api/reseller/customers` (and admin `POST /api/resellers/:id/customers`, same body) now requires `email` and `company_name` alongside `username` + `password`. The reseller asserts the email (same trust as admin CreateUser); it must be a valid address (≤50 chars) not already used by another New API user. After the customer row is created, `ProvisionSpChatOrg` runs with the plaintext password, so the customer signs into SP Chat with the same email + password. If provisioning fails the New API customer is **kept** (not rolled back), the response is still `success: true` with `data.sp_chat_provisioned: false` and `data.sp_chat_warning: "Customer created, but SP Chat provisioning failed"`, and the reseller UI shows a warning toast. The failed attempt cleans up after itself; retry by an admin password reset (below). Customer list/detail responses include `email`, `company_name`, `sp_chat_provisioned`.
- **Admin UpdateUser with a new password**: provisions a user that has `company_name` + email but is not yet linked (deferred / retry path).

Steps: New API token `sp-chat-{userId}` → `POST /api/v1/auths/add` (role `user`) → `POST /api/v1/groups/create` with `data.sp_company` (`newapi_user_id`, `company_name`, `newapi_token_id`, `admin_user_id`) → add user to group → persist link fields.

Idempotency: per-user lock; already-linked users are a no-op. Any step failure deletes what that attempt created (group, OWUI user, token) and leaves `sp_chat_provisioned=0`, so a retry starts clean. Errors are logged; end users never see raw SP Chat responses.

## Password sync
For linked users, the new password is pushed to `POST /api/v1/users/{owui_admin_user_id}/update` (in background) on: admin UpdateUser password reset, self password change (UpdateSelf), and the email reset flow (`/api/user/reset`).
Reseller customers use the same three paths (they are ordinary users with `reseller_id > 0`). The reseller console has no customer password-reset endpoint, so a reseller cannot change a customer's password; customers change their own, or an admin resets it.

Username / display name stay `max=20` (unchanged); the reseller create form enforces the same `maxLength` (username 20, display name 20, email 50, company name 128).

## SP Chat COPY org-admin
`/api/v1/sp-org/*` (COPY backend `routers/sp_org.py`): the account in `data.sp_company.admin_user_id` can list members, create new member accounts (role `user`, added to the company group only — existing accounts cannot be attached), and remove members (removed users with no remaining group are set to `pending`). `/admin` in the COPY shows this panel for org admins.

## DB columns (`users`)
`company_name`, `owui_group_id`, `owui_admin_user_id`, `newapi_token_id`, `sp_chat_provisioned`, `sp_chat_provisioned_at`

## OWUI role
Org admin uses OWUI role **`user`** (never global `admin`). Isolation via group + `group.data.sp_company`.
