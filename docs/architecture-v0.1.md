# v0.1 command and INWX provider contract

Status: accepted for implementation on 2026-07-26.

This document is the executable product contract for `inwx` v0.1. A command
that behaves differently is a bug unless this contract is deliberately revised.

`inwx` is an unofficial community CLI. It is not affiliated with, endorsed by,
maintained by, or supported by INWX GmbH. Official services and support are
available at <https://www.inwx.com/>.

## Scope

v0.1 manages individual A, AAAA, CNAME, TXT, and MX records in existing INWX
nameserver zones. Read-only inventory includes every record type returned by
INWX so unsupported existing data is visible and can be preserved.

It does not register or transfer domains, manage contacts, billing, DNSSEC,
nameserver delegation, bulk desired state, ACME, background reconciliation, or
provider accounts. Future capabilities may add another top-level command, but
v0.1 contains no abstractions or placeholder commands for them.

## Client decision

The CLI uses a small direct INWX JSON-RPC client built from the Go standard
library. It does not use `github.com/libdns/inwx` at runtime.

The maintained provider was evaluated at commit
`1493d22fe1f9d1975c2110127087b89808c12fbf` (2026-03-16). It correctly
implements authentication, TOTP, zone pagination, and common record conversion,
but cannot satisfy this CLI's mutation safety contract:

- `GetRecords` receives provider record IDs and discards them when converting
  to `libdns.Record`.
- `SetRecords` selects by name and type. It creates a record when none exists
  and rejects duplicates only after lookup.
- `DeleteRecords` selects by name, type, and content and deletes every matching
  record.
- The provider owns its unexported transport, so an ID-preserving extension
  would duplicate authentication and record requests.

Using the provider plus a side client would therefore add a dependency and two
stateful sessions without retaining its main abstraction. The direct client is
limited to `account.login`, `account.unlock`, `account.logout`,
`nameserver.list`, `nameserver.info`, `nameserver.createRecord`,
`nameserver.updateRecord`, and `nameserver.deleteRecord`.

The client follows INWX's JSON-RPC envelope rather than JSON-RPC 2.0: requests
contain `method` and `params`; authentication state is held in the HTTP cookie
jar. Response codes from 1000 through 1500 inclusive are successful. Every
other code is an API error and retains `code`, `msg`, `reasonCode`, and
`reason` after redaction.

Sources:

- <https://github.com/libdns/inwx>
- <https://github.com/inwx/python-client>
- <https://www.inwx.com/en/offer/api>

## Command grammar

Global flags precede the command:

```text
inwx [--json] [--environment production|ote] [--timeout 20s] [--retries 2] <command>
```

Commands:

```text
inwx version
inwx auth check
inwx dns zones list
inwx dns records list <zone> [--type A|AAAA|CNAME|TXT|MX] [--name <name>]
inwx dns records create <zone> --type <type> --name <name> --value <value>
    [--ttl <seconds>] [--priority <0..65535>] [--expect <token>] [--apply]
inwx dns records update <zone> --id <id>
    [--name <name>] [--value <value>] [--ttl <seconds>]
    [--priority <0..65535>] [--expect <token>] [--apply]
inwx dns records delete <zone> --id <id> [--expect <token>] [--apply]
```

`--help` is valid at the root, group, and command levels. `-h` aliases
`--help`. There are no other short flags. Unknown flags, extra positional
arguments, missing values, and flags placed before the wrong command fail with
exit 2.

The default TTL for create is 3600 seconds. Update preserves every omitted
field. Update requires at least one changed field after normalization. Priority
is required for MX create, optional for MX update, and invalid for other types.

`--value-file <path>` and `--value-stdin` are mutually exclusive alternatives
to `--value`. They exist for automation that must keep transient DNS values out
of process arguments. Exactly one value source is required for create. At most
one is accepted for update.

## Environment and credentials

The environment is selected in this order:

1. `--environment`
2. `INWX_ENVIRONMENT`
3. `production`

Only `production` and `ote` are valid. They map to fixed endpoints:

```text
production  https://api.domrobot.com/jsonrpc/
ote         https://api.ote.domrobot.com/jsonrpc/
```

The public CLI accepts no arbitrary endpoint. Tests inject an HTTP handler
internally.

Credentials are read independently for username, password, and optional shared
secret:

```text
INWX_USERNAME          INWX_USERNAME_FILE
INWX_PASSWORD          INWX_PASSWORD_FILE
INWX_SHARED_SECRET     INWX_SHARED_SECRET_FILE
```

For each pair, setting both variables is a configuration error. `_FILE` does
not silently override an environment value. A credential file must be a
regular file no larger than 64 KiB. One trailing LF or CRLF is removed; other
bytes are preserved. Empty username and password values fail before any
network request. The shared secret is optional unless INWX requests
`GOOGLE-AUTH`.

Passwords and TOTP shared secrets have no command-line flags, config files, or
debug representation. Error text redacts all loaded credential values. The CLI
does not load `.env` files.

## Transport, timeout, and retry rules

The default command deadline is 20 seconds. `--timeout` accepts Go duration
syntax from one second through five minutes. SIGINT cancels the context and
exits 130.

`--retries` accepts 0 through 5 and defaults to 2. Read-only calls may retry
after a transport error that produced no response, HTTP 429, or HTTP 502, 503,
or 504. Delays are deterministic: 250 ms, 1 s, then 2 s, capped at 2 s.
`Retry-After` is honored when it is an integer number of seconds within the
remaining command deadline.

Login may retry under the same rules. Logout is best effort and never hides the
command's primary result.

Create, update, and delete calls never retry automatically. A lost mutation
response has an unknown outcome; the CLI re-reads state and reports either a
verified result, a verification failure, or an API/transport error. It never
submits a second mutation.

The HTTP client:

- uses a private cookie jar and no ambient browser cookies;
- requires HTTPS for fixed public endpoints;
- rejects non-2xx HTTP status after applying the retry rules;
- limits response bodies to 4 MiB;
- closes every response body;
- rejects malformed JSON and missing response codes.

## Normalized DNS model

JSON record IDs are strings even when INWX serializes a numeric-looking value.
Every record contains:

```json
{
  "id": "12345",
  "zone": "example.com.",
  "name": "@",
  "fqdn": "example.com.",
  "type": "MX",
  "value": "mail.example.net.",
  "ttl": 3600,
  "priority": 10
}
```

Normalization rules:

- Zones are converted with the IDNA lookup profile, lower-cased, and emitted
  with one trailing dot.
- A record name may be `@`, relative to the zone, or a fully-qualified name
  inside the zone. The canonical `name` is `@` or a lower-case relative name.
  `fqdn` always has one trailing dot.
- A name outside the selected zone is invalid. Empty labels, consecutive dots,
  and wildcard names are invalid in v0.1.
- A, AAAA, CNAME, and MX values are parsed by type. Addresses use canonical
  `netip` text. CNAME and MX targets use IDNA, lower case, and one trailing dot.
- TXT input is the exact unquoted text. Surrounding quote characters are data,
  not zone-file syntax, and are not stripped. Newlines and NUL bytes are
  rejected. TXT values are never IDNA-normalized.
- Types are emitted as upper-case text. Read-only inventory passes other INWX
  types through with their API content unchanged. Create and update are limited
  to A, AAAA, CNAME, TXT, and MX.
- TTL is an integer from 300 through 2147483647 seconds. Values below INWX's
  documented minimum are rejected rather than silently increased.
- MX priority is an integer from 0 through 65535. Read-only foreign types retain
  a non-zero API priority; priority is otherwise omitted from JSON. Mutations
  accept priority only for MX.
- Output is sorted by canonical name, type, priority, value, then ID.

The API representation uses the absolute name without a trailing dot and
separate `content`, `ttl`, and `prio` fields. Conversion occurs only at the API
boundary.

## Pagination and completeness

`nameserver.list` starts at page 1 with `pagelimit=100` and continues until the
number of distinct returned zones equals `resData.count`. An empty page before
that point, a repeated page, a changing total count, or more than 10,000 zones
is an API error.

`nameserver.info` returns `resData.count` and `resData.record` without a
documented paging contract. The CLI requires the count to equal the record
array length and refuses to act on a partial response.

Duplicate zones or record IDs in one response are API errors.

## Output contract

Human output is a deterministic table on stdout. Diagnostics and errors go to
stderr. Color is never required to interpret output and is disabled when stdout
is not a terminal.

`--json` emits one compact JSON object followed by LF. Successful data goes to
stdout; errors use the same envelope on stderr and leave stdout empty.

Success:

```json
{
  "schema_version": "inwx.cli/v1",
  "command": "dns.records.list",
  "environment": "ote",
  "ok": true,
  "data": {}
}
```

Error:

```json
{
  "schema_version": "inwx.cli/v1",
  "command": "dns.records.update",
  "environment": "ote",
  "ok": false,
  "error": {
    "kind": "conflict",
    "message": "record changed since preview"
  }
}
```

Keys shown above are mandatory. Command-specific `data` is an object, including
for lists. Optional error fields are `api_code`, `reason_code`, and `details`.
No error field contains credentials, cookies, raw HTTP requests, or raw
responses.

Exit codes:

```text
0    success, including a preview that made no mutation
2    command usage or local DNS validation error
3    credential, environment, or authentication error
4    INWX API, HTTP, timeout, or response-shape error
5    mutation precondition conflict
6    post-mutation verification failed
130  interrupted by SIGINT
```

## Mutation protocol

Mutations are never interactive and are never presented as transactional.
Each command affects exactly one record.

### Preview

Without `--apply`, create, update, and delete:

1. authenticate;
2. fetch the complete zone;
3. resolve an update or delete only by exact ID;
4. validate DNS constraints and duplicates;
5. emit a deterministic before/after diff;
6. emit `applied: false` and an `expect` token;
7. exit 0 without a mutation call.

The token is lower-case hex SHA-256 over the JSON encoding of:

```text
inwx.cli/v1 + NUL + environment + NUL + zone + NUL + operation + NUL
    + canonical relevant current state + NUL + canonical requested state
```

Object keys and record arrays use the canonical order defined above. The
relevant current state is the exact ID record for update/delete. For create it
is every record at the owner name, because CNAME exclusivity can conflict with
other types.

### Apply

`--apply` requires `--expect`. Apply:

1. repeats the full preview read and normalization;
2. recomputes the token and compares it in constant time;
3. exits 5 without mutation when it differs;
4. prints/emits the same diff;
5. submits exactly one mutation request;
6. re-reads the complete zone;
7. verifies the requested record state;
8. emits `applied: true`, `verified: true`, and the final record or deleted ID.

Create conflicts when an identical canonical record already exists. It also
rejects CNAME coexistence: a CNAME owner cannot have another CNAME or any other
record type, and no CNAME may be added where other records exist.

Update and delete fail when the ID is absent or duplicated. Update preserves
the ID and every omitted field. Provider-managed fields not modeled by v0.1 are
never sent. A request that normalizes to no change is a successful no-op
preview; `--apply` sends no mutation and reports `applied: false`.

Delete requires the exact ID both in preview and apply. Name, type, or content
never select a destructive target.

Verification is against the INWX control-plane API. Output includes canonical
`fqdn`, type, value, and the selected environment so an operator can perform an
authoritative DNS query separately. DNS propagation is not claimed to be
instantaneous.

## Live OT&E evidence

On 2026-07-26 a credential-redacted probe against
`https://api.ote.domrobot.com/jsonrpc/` observed:

- `account.login`: code 1000;
- `nameserver.list`: code 1000 with a zero-zone new account;
- `account.logout`: code 1500.

After an exact preview, a second probe with an explicit `--apply` created one
uniquely named disposable OT&E MASTER zone and representative A, AAAA, CNAME,
TXT, and MX records using documentation-only values. `nameserver.info`
returned all five records with stable string-convertible IDs and exactly these
fields:

```text
content id name prio ttl type
```

The probe compared every name, type, content, TTL, priority, and returned ID,
then deleted the complete disposable zone. A final `nameserver.list` verified
cleanup. No production endpoint was called, and the OT&E account returned to
zero zones.

Credential-free response fixtures live in `internal/inwx/testdata`. They model
string record IDs and the complete required fields for A, AAAA, CNAME, TXT, and
MX.

The implemented CLI was then exercised against a fresh disposable OT&E zone.
For one unique TXT record, create, update, and delete each ran as a separate
preview and apply with the preview's exact token. Every apply used one mutation
request, re-read the complete zone, and verified the requested state. The
record and disposable zone were deleted, and a final zone listing verified
that the account again contained zero zones.
