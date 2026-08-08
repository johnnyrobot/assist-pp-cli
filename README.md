# ASSIST CLI

Anonymous command-line access to the public [ASSIST](https://assist.org) articulation and transferability data used by the website.

No API key, account, or licensed API access is required. The client starts an anonymous public-site session and automatically handles ASSIST's anti-forgery cookie/header handshake in memory.

## Install

`assist-pp-cli` is all you need. The separate `assist-pp-mcp` server is only for chat apps that can't run shell commands.

### Go install

Requires Go 1.26.5 or newer:

```bash
go install github.com/johnnyrobot/assist-pp-cli/cmd/assist-pp-cli@latest
```

To also install the MCP server binary:

```bash
go install github.com/johnnyrobot/assist-pp-cli/cmd/assist-pp-mcp@latest
```

Both install into `$GOPATH/bin` (default `$HOME/go/bin`) — make sure that directory is on `$PATH`.

### Pre-built binary

Download a binary for your platform from the [latest release](https://github.com/johnnyrobot/assist-pp-cli/releases/latest).

On macOS, clear the Gatekeeper quarantine after downloading:

```bash
xattr -d com.apple.quarantine assist-pp-cli
```

On Unix, mark it executable:

```bash
chmod +x assist-pp-cli
```

### From source

```bash
git clone https://github.com/johnnyrobot/assist-pp-cli.git
cd assist-pp-cli
mkdir -p "$HOME/.local/bin"
go build -trimpath -o "$HOME/.local/bin/assist-pp-cli" ./cmd/assist-pp-cli
go build -trimpath -o "$HOME/.local/bin/assist-pp-mcp" ./cmd/assist-pp-mcp
```

### Use with Claude Desktop

An MCP host needs only the binary path — there are no credential environment variables:

```json
{
  "mcpServers": {
    "assist": {
      "command": "/home/you/.local/bin/assist-pp-mcp"
    }
  }
}
```

## Quick start

```bash
assist-pp-cli doctor --json
assist-pp-cli settings --json
assist-pp-cli academic-years --json
assist-pp-cli institutions --json

# Public articulation discovery.
assist-pp-cli agreements institutions 110 --json
assist-pp-cli agreements categories 7 110 74 --json
assist-pp-cli agreements list 7 110 74 --types Department --json
assist-pp-cli agreements get --key '74/110/to/7/Department/13008' --json

# Public transferability lists.
assist-pp-cli transferability years 110 --json
assist-pp-cli transferability categories 110 74 UCTEL --json
assist-pp-cli transferability courses 110 74 UCTEL --json
```

Human-readable workflows are also available:

```bash
assist-pp-cli resolve institution "Diablo Valley College" --year "2024-2025" --agent
assist-pp-cli advisor agreements \
  --sending "Diablo Valley College" \
  --receiving "UC San Diego" \
  --year "2023-2024" \
  --types Major,Department \
  --agent
```

`agreements diff` compares two public agreement payloads by composite key. Add `--agent` to any command for compact JSON and non-interactive defaults.

## Unique Features

Things the assist.org website doesn't give you:

- **`resolve institution`** — turn a human name and catalog year into the exact numeric IDs every other endpoint requires, in one call. The site makes you click through dropdowns to discover them.
- **`advisor agreements`** — go from two institution names and a year straight to the matching agreements, collapsing the resolve → institutions → categories → list chain into a single command.
- **`agreements diff`** — compare two agreement payloads by composite key and get deterministic added / removed / changed JSON paths. There is no way to diff catalog years on the site.
- **`workflow archive`** — mirror every resource into a local store for offline use, with `workflow status` reporting sync state per resource.
- **Composite agreement keys** — `agreements list` returns stable keys like `75/118/to/1/Department/5080` that `agreements get` accepts directly, so an agreement is addressable and scriptable.

## Recipes

Every command below is a real, working call — IDs are live as of writing.

**Find every agreement between two colleges for a catalog year**

```bash
# 1. Names -> IDs (East Los Angeles College, 2024-2025)
assist-pp-cli resolve institution "East Los Angeles College" --year "2024-2025" --agent
#    -> institution_id 118, academic_year_id 75

# 2. Which institutions have published agreements with ELAC?
assist-pp-cli agreements institutions 118 --agent

# 3. What agreement categories exist for this pair and year?
#    Argument order is: <receiving> <sending> <academicYear>
assist-pp-cli agreements categories 1 118 75 --agent

# 4. List the department agreements
assist-pp-cli agreements list 1 118 75 --types Department --agent
#    -> reports[].key, e.g. "75/118/to/1/Department/5080"

# 5. Fetch one by its composite key
assist-pp-cli agreements get --key '75/118/to/1/Department/5080' --agent
```

Steps 1–5 collapse into one call when you know the names:

```bash
assist-pp-cli advisor agreements \
  --sending "East Los Angeles College" \
  --receiving "California Maritime Academy" \
  --year "2024-2025" \
  --types Major,Department \
  --agent
```

**Check what transfers from a college to CSU/UC**

```bash
assist-pp-cli transferability years 118 --agent                  # which years have lists
assist-pp-cli transferability categories 118 76 CALGETC --agent  # groupings for that list
assist-pp-cli transferability courses 118 76 CALGETC --agent     # the course rows
```

**See what changed between two catalog years**

`agreements diff` takes two composite keys positionally, `<from-key> <to-key>`, fetches both payloads live, and reports additions, removals and changes as RFC 6901 JSON Pointer paths:

```bash
assist-pp-cli agreements diff \
  '74/118/to/1/Department/5080' \
  '75/118/to/1/Department/5080' \
  --agent
```

**Work offline**

```bash
assist-pp-cli workflow archive          # mirror all resources locally
assist-pp-cli workflow status           # per-resource sync state
assist-pp-cli institutions --data-source local --agent
```

**Narrow the output**

```bash
assist-pp-cli institutions --select id,code --agent      # only the fields you need
assist-pp-cli institutions --compact --agent             # key fields only, fewer tokens
assist-pp-cli institutions --csv > institutions.csv      # spreadsheet
assist-pp-cli agreements list 1 118 75 --types Department --dry-run   # show the request, send nothing
```

## Commands

| Command | What it does |
|---|---|
| `academic-years` | List active academic years (79 as of writing; `id` is what other commands take). |
| `institutions` | List all institutions with codes, names and history (181 as of writing). |
| `settings` | Public ASSIST application limits and version settings. |
| `resolve institution` | Name + year → exact institution and academic-year IDs. Requires `--year`. |
| `agreements institutions` | Institutions that have published agreements with a given institution. |
| `agreements categories` | Agreement categories for a `<receiving> <sending> <year>` triple. |
| `agreements list` | Agreements for that triple. `--types` is required (`Prefix`, `Department`, `Major`, `GeneralEducation`). Returns composite keys. |
| `agreements get` | Fetch one agreement by `--key`. |
| `agreements diff` | Deterministic added / removed / changed paths between two agreement payloads. |
| `advisor agreements` | Names + year + types → matching agreements in one call. |
| `transferability years` | Academic years with transferability lists for an institution. |
| `transferability categories` | Groupings for `<institution> <year> <listType>`. |
| `transferability courses` | Course rows for that list. |
| `workflow archive` / `status` | Mirror all resources locally; report sync state. |
| `doctor` | Verify config, the anonymous handshake and API reachability. |
| `which "<capability>"` | Find the command that implements a capability. |
| `agent-context` | Structured JSON describing the whole CLI, for agents. |
| `profile` | Save and reuse named sets of flags. |
| `recall` / `teach` / `learnings` / `playbook` | Local teach-and-recall loop (see Agent Usage). |

Run `assist-pp-cli <command> --help` for exact arguments — several take positional IDs in a specific order.

## Output Formats

Default is a human table on a TTY and JSON when piped.

| Flag | Effect |
|---|---|
| `--json` | JSON output |
| `--csv` | CSV, for table and array responses |
| `--plain` | Tab-separated text |
| `--quiet` | Bare output, one value per line |
| `--compact` | Only key fields — id, name, status, timestamps |
| `--select id,name` | Only the named fields |
| `--human-friendly` | Force color and rich formatting |
| `--deliver file:<path>` | Route output to a file or `webhook:<url>` instead of stdout |

Responses carry a `meta` block recording `auth`, the `endpoint` hit, and `source` (`live` or `local`), so you can tell where an answer came from.

## Agent Usage

`--agent` sets every agent-friendly default at once: `--json --compact --no-input --no-color --yes`.

```bash
assist-pp-cli agent-context --pretty        # full machine-readable CLI description
assist-pp-cli which "find transfer agreements" --agent
```

The CLI ships a local teach-and-recall loop. `recall` checks prior learnings before you spend calls on discovery, `teach` records a query → resource mapping, and `learnings` / `playbook` inspect what has accumulated. Everything stays in the local store — nothing is uploaded. Disable it per-invocation with `--no-learn` or for a whole session with `ASSIST_NO_LEARN=true`.

Exit codes: `0` ok, `1` unclassified error, `2` usage, `3` not found, `4` auth, `5` API error, `6` partial failure, `7` rate limited, `10` config error. Errors go to stderr. `--allow-partial-failure` downgrades a `6` to a warning.

## Paths & environment variables

Config, data, state and cache resolve independently, each defaulting to the platform location. `doctor` prints all four.

```
config  ~/.config/assist-pp-cli
data    ~/.local/share/assist-pp-cli
state   ~/.local/state/assist-pp-cli
cache   ~/.cache/assist-pp-cli
```

| Variable | Purpose |
|---|---|
| `ASSIST_HOME` | Relocate all four kinds at once |
| `ASSIST_CONFIG_DIR` / `ASSIST_DATA_DIR` / `ASSIST_STATE_DIR` / `ASSIST_CACHE_DIR` | Relocate one kind |
| `ASSIST_CONFIG` | Path to a specific config file |
| `ASSIST_BASE_URL` | Point at a mock server for testing; leave unset for normal use |
| `ASSIST_NO_LEARN` | Disable the teach/recall loop for the session |
| `ASSIST_NO_AUTO_REFRESH` | Disable automatic local-store refresh |

Useful global flags: `--home`, `--config`, `--timeout`, `--rate-limit`, `--no-cache`, `--max-age`, `--data-source auto|live|local`.

## Health Check

```bash
assist-pp-cli doctor          # human summary
assist-pp-cli doctor --json   # machine-readable
```

Checks config, auth mode, the anonymous XSRF handshake, API reachability, cache state and all four resolved paths. Run it first whenever something behaves unexpectedly.

## Public endpoint contract

The CLI mirrors the same anonymous routes used by the current assist.org web application:

- `/api/appsettings`
- `/api/AcademicYears`
- `/api/institutions`
- `/api/institutions/{id}/agreements`
- `/api/agreements/categories`
- `/api/agreements`
- `/api/articulation/Agreements`
- `/api/institutions/{id}/transferability/availableAcademicYears`
- `/api/transferability/categories`
- `/api/transferability/courses`

The site issues `X-XSRF-TOKEN` during an anonymous landing-page request. The CLI retains that cookie in an in-memory jar and echoes it in the matching request header. This is an anti-forgery mechanism, not authentication.

`ASSIST_BASE_URL` remains available for local mock servers and testing. Normal use should keep the default `https://assist.org`.

## Troubleshooting

- Run `assist-pp-cli doctor --json` to verify the anonymous handshake and `/api/AcademicYears` request.
- Use current IDs from `academic-years` and `institutions`; ASSIST agreement availability varies by institution pair and year.
- A `400 Bad Request` from a hand-written HTTP client usually means it skipped the public XSRF bootstrap. The CLI handles this automatically.

ASSIST is an advising aid, not an admission guarantee or transcript evaluation service.
