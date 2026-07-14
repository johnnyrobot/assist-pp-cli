---
name: pp-assist
description: "Use ASSIST's anonymous public endpoints for California articulation agreements, academic years, institutions, and transferability course lists. No API key is required."
author: "johnnyrobot"
license: "Apache-2.0"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
regions: ["US"]
api_language: "en"
metadata:
  openclaw:
    requires:
      bins: [assist-pp-cli]
---

# ASSIST public CLI

Use this CLI for public California transfer and articulation research. Do not use it to guarantee admission, evaluate a transcript, or store student records.

## Required setup

Verify the local binary before invoking it:

```bash
assist-pp-cli --version
assist-pp-cli doctor --json
```

No API key or auth command is required. The CLI automatically creates an anonymous assist.org session and performs the public site's `X-XSRF-TOKEN` cookie/header handshake.

## Workflow

Start with runtime discovery when the exact command is unclear:

```bash
assist-pp-cli which "find an articulation agreement" --json
assist-pp-cli <command> --help
```

Resolve names and years when the user did not supply IDs:

```bash
assist-pp-cli resolve institution "Diablo Valley College" --year "2024-2025" --agent
```

For an institution pair:

```bash
assist-pp-cli agreements categories 7 110 74 --agent
assist-pp-cli agreements list 7 110 74 --types Department --agent
assist-pp-cli agreements get --key '74/110/to/7/Department/13008' --agent
```

For transferability lists:

```bash
assist-pp-cli transferability years 110 --agent
assist-pp-cli transferability categories 110 74 UCTEL --agent
assist-pp-cli transferability courses 110 74 UCTEL --agent
```

For human inputs across multiple public calls:

```bash
assist-pp-cli advisor agreements \
  --sending "Diablo Valley College" \
  --receiving "UC San Diego" \
  --year "2023-2024" \
  --types Major,Department \
  --agent
```

Add `--agent` for compact JSON, no prompts, no color, and confirmation-safe defaults. Use `--select` to constrain verbose agreement or course-list payloads.

Teach only structural, identifier-free queries to the local recall loop; never include student information.
