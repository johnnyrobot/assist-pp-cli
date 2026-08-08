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
