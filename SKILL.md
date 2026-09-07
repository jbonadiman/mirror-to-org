---
name: "mirror-to-org"
description: Mirror GitHub/GitLab/Forgejo repositories into owner orgs on a Forgejo instance, creating each org from the owner's public profile when it doesn't exist yet.
version: 1.0.0
category: devops
tags:
  - gitea
  - forgejo
  - github
  - gitlab
  - mirror
  - cli
commands:
  - group: mirror
    description: Mirror one or more repositories into owner-named orgs on the target instance
    subcommands:
      - name: run
        description: "Usage: mirror-to-org [--target URL] REPO [REPO ...]"
        options: ["--target"]
---

# mirror-to-org

Given one or more repository references, files each one into an
organization named after its owner on a target Forgejo instance,
creating that organization from the owner's public profile when it
doesn't already exist. Repositories land as live pull mirrors carrying
their wiki.

## Quick Start

```bash
# Install
go install github.com/jbonadiman/mirror-to-org@latest

# Configure
export GITEA_TOKEN=...            # org create + repo migration rights on the target
export GITEA_TARGET=https://your-forgejo-instance.example

# Mirror one or more repos
mirror-to-org github.com/kepano/obsidian-minimal
mirror-to-org github.com/kepano/obsidian-minimal gitlab.com/some/repo

# Or pass the target inline instead of via env var
mirror-to-org --target https://your-forgejo-instance.example github.com/kepano/obsidian-minimal
```

A reference is a full URL or a `host/owner/repo` shorthand.

## Agent Usage Guidance

This is a single-purpose, non-interactive CLI with no subcommands or
output-format flags — every invocation is a "mirror these repos" call.

### Configuration

`$GITEA_TOKEN` is always required. Target instance resolution: `--target`
takes precedence over `$GITEA_TARGET`; if neither is set, the CLI exits
2 before doing any work.

| Flag / variable  | Required | Description                                    |
|-------------------|----------|-------------------------------------------------|
| `--target URL`    | one of `--target` or `$GITEA_TARGET` | Target instance base URL (must be `https://`) |
| `$GITEA_TARGET`   | ^        | Same, as an env var                             |
| `$GITEA_TOKEN`    | yes      | Token with org create and repository migration rights on the target |

### Batching

Pass multiple `REPO` arguments in one invocation rather than shelling
out per repo — the CLI processes them independently and reports a
non-zero exit if any one fails, without aborting the rest.

### Exit Codes

Agents should branch on exit code, not stdout/stderr parsing:

| Code | Meaning |
|------|---------|
| 0    | All references mirrored or skipped without error |
| 1    | A runtime failure, or the target rejected the token |
| 2    | Usage error (bad flags, no references, no target configured) |

An exit code of 1 after any progress output means some repos may have
mirrored successfully before the failure — check the printed per-repo
lines, not just the final exit code, to know which ones landed.
