---
okf_version: "0.2"
---
# Knot scripting for AI assistants

How an AI assistant writes and runs its own scripts against the knot API with the `run_script` tool, and the `knot.*` libraries those scripts may import. `script_docs` serves these pages.

## Guide

- [Writing scripts](writing-scripts.md) — the rules, read and write mode (exactly `"read"` or `"write"`), what is never allowed, path rules, what is redacted and how redacted values are written back, the time, call, concurrency, memory and output limits, how to return an answer and what comes back, and worked examples.

## Libraries

- [knot.apiclient](libraries/apiclient.md) — direct calls to the knot API, for anything the libraries below do not cover.
- [knot.audit](libraries/audit.md) — the audit log.
- [knot.event_sink](libraries/event_sink.md) — event sinks: webhooks and scripts that receive knot events.
- [knot.files](libraries/files.md) — file storage buckets and their files.
- [knot.group](libraries/group.md) — user groups.
- [knot.identity](libraries/identity.md) — the user the script runs as.
- [knot.jobs](libraries/jobs.md) — scheduled jobs of spaces.
- [knot.permission](libraries/permission.md) — the permission catalogue.
- [knot.pool](libraries/pool.md) — space pools and leases.
- [knot.role](libraries/role.md) — roles.
- [knot.script](libraries/script.md) — stored scripts.
- [knot.server](libraries/server.md) — server information, cluster and search.
- [knot.skill](libraries/skill.md) — skills.
- [knot.slash_command](libraries/slash_command.md) — slash commands.
- [knot.space](libraries/space.md) — spaces: list, inspect, start, stop, files and commands.
- [knot.stack](libraries/stack.md) — stacks and stack definitions.
- [knot.template](libraries/template.md) — space templates.
- [knot.tunnel](libraries/tunnel.md) — the user's public tunnels.
- [knot.user](libraries/user.md) — users and quotas.
- [knot.vars](libraries/vars.md) — template variables.
- [knot.volume](libraries/volume.md) — volumes.
