# F-03 Demo — User Authentication & Multi-Tenancy

This demo exercises the full auth stack added in F-03: JWT login, invite-only
registration, role-based access control, per-member permission scopes, and
organisation-scoped data isolation.

---

## Prerequisites

### `.env` — set required variables

```bash
# .env additions (copy from .env.example)
JWT_SECRET=demo-secret-that-is-at-least-32-characters-long
BOOTSTRAP_ADMIN_EMAIL=admin@cogniflow.dev
BOOTSTRAP_ADMIN_PASSWORD=adminpass123
```

### Start the stack

```bash
docker compose up --build
```

The server prints `bootstrapped system_admin email=admin@cogniflow.dev` on
first startup when no users exist yet.

```bash
BASE=http://localhost:8080/v1
```

---

## Scenario 1 — Bootstrap and first login

### 1a — Unauthenticated request returns 401

Every protected endpoint requires a valid Bearer token:

```bash
curl -s $BASE/workflows | jq .
```

**Expected:**

```json
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "missing or invalid Authorization header"
  }
}
```

### 1b — Login as the bootstrapped system_admin

```bash
ADMIN_TOKEN=$(curl -s -X POST $BASE/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@cogniflow.dev","password":"adminpass123"}' \
  | jq -r '.token')

echo "Token: $ADMIN_TOKEN"
```

**Expected:** a JWT string.

### 1c — Inspect the current user

```bash
curl -s $BASE/auth/me \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

**Expected:**

```json
{
  "id": "...",
  "org_id": "...",
  "org_name": "Default",
  "email": "admin@cogniflow.dev",
  "role": "system_admin",
  "permissions": ["workflow:read","workflow:write","workflow:run","eval:read","eval:write","eval:run"],
  "created_at": "..."
}
```

### 1d — Wrong password returns same error (no enumeration)

```bash
curl -s -X POST $BASE/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@cogniflow.dev","password":"wrongpass"}' | jq .error
```

```bash
curl -s -X POST $BASE/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"nobody@example.com","password":"whatever"}' | jq .error
```

**Expected (both):**

```json
{
  "code": "UNAUTHORIZED",
  "message": "invalid email or password"
}
```

---

## Scenario 2 — Org management (system_admin)

### 2a — Create a second organisation

```bash
ORG2=$(curl -s -X POST $BASE/admin/orgs \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Acme Corp",
    "admin_email": "alice@acme.com",
    "admin_password": "acmepass123"
  }')

ORG2_ID=$(echo $ORG2 | jq -r '.organization.id')
ALICE_ID=$(echo $ORG2 | jq -r '.admin.id')
echo "Org: $ORG2_ID  Admin: $ALICE_ID"
```

**Expected:** org + admin user returned, role `org_admin`.

### 2b — List all organisations

```bash
curl -s $BASE/admin/orgs \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq '[.organizations[] | {name,id}]'
```

**Expected:** two orgs — `Default` and `Acme Corp`.

### 2c — Non-system_admin cannot access org admin routes

```bash
ALICE_TOKEN=$(curl -s -X POST $BASE/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@acme.com","password":"acmepass123"}' | jq -r '.token')

curl -s $BASE/admin/orgs \
  -H "Authorization: Bearer $ALICE_TOKEN" | jq .error.code
```

**Expected:** `"FORBIDDEN"`

---

## Scenario 3 — Invite flow (org_admin)

### 3a — Invite a member to Acme Corp

```bash
INVITE=$(curl -s -X POST $BASE/org/users/invite \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "email": "bob@acme.com",
    "role": "member",
    "permissions": ["workflow:read","workflow:run"]
  }')

INVITE_TOKEN=$(echo $INVITE | jq -r '.token')
echo "Invite token: $INVITE_TOKEN"
```

**Expected:** a `token` field in the response (in production this would be emailed).

### 3b — Preview the invitation (public endpoint, no auth)

```bash
curl -s "$BASE/auth/invite/$INVITE_TOKEN" | jq .
```

**Expected:**

```json
{
  "email": "bob@acme.com",
  "role": "member",
  "org_name": "Acme Corp"
}
```

### 3c — Accept the invitation and create an account

```bash
BOB=$(curl -s -X POST $BASE/auth/accept-invite \
  -H 'Content-Type: application/json' \
  -d "{\"token\":\"$INVITE_TOKEN\",\"password\":\"bobpass123\"}")

BOB_TOKEN=$(echo $BOB | jq -r '.token')
BOB_ID=$(echo $BOB | jq -r '.user.id')
echo "Bob token: $BOB_TOKEN"
echo "Bob perms: $(echo $BOB | jq -r '.user.permissions')"
```

**Expected:** `permissions` contains only `workflow:read` and `workflow:run` (the restricted set from the invite).

### 3d — Reusing an accepted token is rejected

```bash
curl -s -X POST $BASE/auth/accept-invite \
  -H 'Content-Type: application/json' \
  -d "{\"token\":\"$INVITE_TOKEN\",\"password\":\"anotherpass123\"}" | jq .error.code
```

**Expected:** `"INVITATION_USED"`

---

## Scenario 4 — Permission scope enforcement (member)

Bob has `workflow:read` and `workflow:run` but NOT `workflow:write`.

### 4a — Bob can list workflows

```bash
curl -s $BASE/workflows \
  -H "Authorization: Bearer $BOB_TOKEN" | jq '.workflows | length'
```

**Expected:** `0` (Acme Corp has no workflows yet — and Bob cannot see Default org workflows).

### 4b — Bob cannot create a workflow (missing workflow:write)

```bash
curl -s -X POST $BASE/workflows \
  -H "Authorization: Bearer $BOB_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Bob sneaky workflow","trigger":{"kind":"manual"},"nodes":[],"edges":[]}' \
  | jq .error
```

**Expected:**

```json
{
  "code": "FORBIDDEN",
  "message": "missing permission: workflow:write"
}
```

### 4c — Alice (org_admin) creates a workflow — Bob can read it

```bash
WF_ID=$(curl -s -X POST $BASE/workflows \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Acme Hello",
    "trigger": {"kind":"manual"},
    "nodes": [
      {
        "id": "req",
        "type_id": "http.request",
        "label": "Ping",
        "position": {"x":0,"y":0},
        "config": {"url":"https://httpbin.org/get","method":"GET"}
      }
    ],
    "edges": []
  }' | jq -r '.id')

echo "Workflow: $WF_ID"

curl -s "$BASE/workflows/$WF_ID" \
  -H "Authorization: Bearer $BOB_TOKEN" | jq '{id,name}'
```

**Expected:** Bob can read the workflow.

### 4d — Bob can trigger a run (has workflow:run)

```bash
RUN_ID=$(curl -s -X POST "$BASE/workflows/$WF_ID/runs" \
  -H "Authorization: Bearer $BOB_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{}' | jq -r '.run_id')

sleep 3
curl -s "$BASE/runs/$RUN_ID" \
  -H "Authorization: Bearer $BOB_TOKEN" | jq '{status,triggered_by}'
```

**Expected:** `status: "succeeded"`, `triggered_by: "manual"`

### 4e — Bob cannot delete the workflow (missing workflow:write)

```bash
curl -s -X DELETE "$BASE/workflows/$WF_ID" \
  -H "Authorization: Bearer $BOB_TOKEN" | jq .error.code
```

**Expected:** `"FORBIDDEN"`

---

## Scenario 5 — Organisation data isolation

The admin user in the Default org cannot see Acme Corp's workflows, and
vice versa.

### 5a — Create a workflow in the Default org

```bash
DEFAULT_WF=$(curl -s -X POST $BASE/workflows \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Default Org Secret Workflow",
    "trigger": {"kind":"manual"},
    "nodes": [],
    "edges": []
  }' | jq -r '.id')

echo "Default workflow: $DEFAULT_WF"
```

### 5b — Alice cannot see Default org workflows

```bash
curl -s $BASE/workflows \
  -H "Authorization: Bearer $ALICE_TOKEN" | jq '[.workflows[].name]'
```

**Expected:** `["Acme Hello"]` — only Acme Corp workflows.

### 5c — system_admin can see all workflows (no org filter in token — sees own org)

The system_admin is in the Default org, so they see Default org workflows:

```bash
curl -s $BASE/workflows \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq '[.workflows[].name]'
```

**Expected:** `["Default Org Secret Workflow"]`

### 5d — Direct cross-org GET is rejected (404, not 403)

Leaking existence of a resource via 403 is also an information leak. The
workflow GET is scoped to the caller's org_id, so a cross-org request
returns 404:

```bash
curl -s "$BASE/workflows/$DEFAULT_WF" \
  -H "Authorization: Bearer $ALICE_TOKEN" | jq .error.code
```

**Expected:** `"NOT_FOUND"` — Alice cannot confirm the workflow exists.

---

## Scenario 6 — Role and permission management (org_admin)

### 6a — List Acme Corp users

```bash
curl -s $BASE/org/users \
  -H "Authorization: Bearer $ALICE_TOKEN" | jq '[.users[] | {email,role,permissions}]'
```

**Expected:** Alice (org_admin, all perms) and Bob (member, read+run only).

### 6b — Grant Bob workflow:write

```bash
curl -s -X PUT "$BASE/org/users/$BOB_ID/permissions" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"permissions":["workflow:read","workflow:write","workflow:run"]}' \
  -o /dev/null -w "%{http_code}\n"
```

**Expected:** `204`

### 6c — Bob can now create a workflow

```bash
curl -s -X POST $BASE/workflows \
  -H "Authorization: Bearer $BOB_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Bobs New Workflow",
    "trigger": {"kind":"manual"},
    "nodes": [],
    "edges": []
  }' | jq '{id,name}'
```

**Expected:** workflow created. Note: Bob's token still carries the old permissions (JWT is stateless). Bob must re-login for the new scopes to take effect in the token. The *next* curl uses a fresh login:

```bash
BOB_TOKEN=$(curl -s -X POST $BASE/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"bob@acme.com","password":"bobpass123"}' | jq -r '.token')

curl -s -X POST $BASE/workflows \
  -H "Authorization: Bearer $BOB_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Bobs New Workflow",
    "trigger": {"kind":"manual"},
    "nodes": [],
    "edges": []
  }' | jq '{id,name}'
```

### 6d — Org_admin cannot promote to system_admin

```bash
curl -s -X PUT "$BASE/org/users/$BOB_ID/role" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"role":"system_admin"}' | jq .error.code
```

**Expected:** `"FORBIDDEN"`

### 6e — system_admin can promote to any role

```bash
curl -s -X PUT "$BASE/admin/users/$BOB_ID/role" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"role":"system_admin"}' \
  -o /dev/null -w "%{http_code}\n"
```

**Expected:** `204`

Restore Bob to member before continuing:

```bash
curl -s -X PUT "$BASE/admin/users/$BOB_ID/role" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"role":"member"}' \
  -o /dev/null -w "%{http_code}\n"
```

---

## Scenario 7 — Webhook trigger (unauthenticated + HMAC)

Webhooks remain publicly callable — external systems fire them without a JWT.

### 7a — Create a webhook-triggered workflow

```bash
WEBHOOK_WF=$(curl -s -X POST $BASE/workflows \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Acme Webhook",
    "trigger": {"kind":"webhook"},
    "timeout_seconds": 30,
    "nodes": [
      {
        "id": "log",
        "type_id": "http.request",
        "label": "Log payload",
        "position": {"x":0,"y":0},
        "config": {
          "url": "https://httpbin.org/post",
          "method": "POST",
          "body": "{{._initial.event}}"
        }
      }
    ],
    "edges": []
  }' | jq -r '.id')

echo "Webhook workflow: $WEBHOOK_WF"
```

### 7b — Fire the webhook without any token

```bash
curl -s -X POST "$BASE/webhooks/$WEBHOOK_WF" \
  -H 'Content-Type: application/json' \
  -d '{"event":"order.placed","customer_id":"cust-42"}' | jq .
```

**Expected:** `{"run_id": "..."}` — a run was dispatched without authentication.

### 7c — Confirm the run succeeded

```bash
sleep 3
curl -s "$BASE/workflows/$WEBHOOK_WF/runs" \
  -H "Authorization: Bearer $ALICE_TOKEN" | \
  jq '[.[] | {status,triggered_by}]'
```

**Expected:** `triggered_by: "webhook"`, `status: "succeeded"`.

---

## Scenario 8 — Self-service safety guards

### 8a — Admin cannot delete themselves

```bash
curl -s -X DELETE "$BASE/admin/users/$ALICE_ID" \
  -H "Authorization: Bearer $ALICE_TOKEN" | jq .error.code
```

```bash
# system_admin also cannot delete themselves
ADMIN_SELF_ID=$(curl -s $BASE/auth/me \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq -r '.id')

curl -s -X DELETE "$BASE/admin/users/$ADMIN_SELF_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .error.code
```

**Expected (both):** `"VALIDATION_FAILED"`

### 8b — Admin cannot change their own role

```bash
curl -s -X PUT "$BASE/org/users/$ALICE_ID/role" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"role":"member"}' | jq .error.code
```

**Expected:** `"VALIDATION_FAILED"`

---

## Scenario 9 — Token expiry

Set a very short TTL to observe expiry. This requires restarting with
`JWT_TTL=5s` in `.env`, then:

```bash
SHORT_TOKEN=$(curl -s -X POST $BASE/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@cogniflow.dev","password":"adminpass123"}' | jq -r '.token')

sleep 6

curl -s $BASE/auth/me \
  -H "Authorization: Bearer $SHORT_TOKEN" | jq .error
```

**Expected:**

```json
{
  "code": "UNAUTHORIZED",
  "message": "invalid or expired token"
}
```

Restore `JWT_TTL=24h` (or remove it to use the default) before continuing.

---

## Scenario 10 — Delete an organisation (system_admin)

Deleting an org cascades: all its users, workflows, eval suites, and RAG
documents are removed.

```bash
# Count Acme's workflows before delete
curl -s $BASE/workflows \
  -H "Authorization: Bearer $ALICE_TOKEN" | jq '.workflows | length'

# Delete the org
curl -s -X DELETE "$BASE/admin/orgs/$ORG2_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -o /dev/null -w "%{http_code}\n"
```

**Expected:** `204`

Alice's token is now invalid (user deleted):

```bash
curl -s $BASE/auth/me \
  -H "Authorization: Bearer $ALICE_TOKEN" | jq .error.code
```

**Expected:** `"UNAUTHORIZED"` (user no longer exists; `GET /auth/me` does a live DB lookup).

---

## Frontend walkthrough

1. **Unauthenticated redirect** — open `http://localhost:3000`. You are redirected to `/login`.
2. **Login** — enter `admin@cogniflow.dev` / `adminpass123`. You are returned to the workflow list. The header shows the email, org name, and a **Sign out** button. Because the role is `system_admin`, **Users** and **Orgs** links appear next to **Grader Plugins**.
3. **Org Users** — click **Users** → `/org/users`. The invite form is at the top. After inviting, copy the link from the response box and paste it in a new tab to reach the **Accept Invite** page.
4. **Accept Invite** — set a password, click **Create account**. You are logged in as the new user and redirected to `/`.
5. **Permission toggles** — as an org_admin, go to **Users**. Member rows show scope pills (`workflow:read`, `workflow:write`, etc.). Clicking a pill toggles the permission on/off immediately.
6. **Sign out** — click **Sign out**. Token is cleared, you are sent back to `/login`.
7. **Admin Orgs** (system_admin only) — log back in as the system admin, click **Orgs** → `/admin/orgs`. Create a second org, then delete it.

---

## Cleanup

```bash
# Remove the Default org workflow created in Scenario 5
curl -s -X DELETE "$BASE/workflows/$DEFAULT_WF" \
  -H "Authorization: Bearer $ADMIN_TOKEN"

docker compose down -v
echo "Done"
```
