---
type: "API Reference"
title: "knot.jobs"
description: "Scheduled job management for spaces."
resource: "https://getknot.dev/reference/libraries/jobs/"
tags:
  - "spaces"
  - "api"
  - "scripting"
  - "automation"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/jobs/"
---
# knot.jobs

The `knot.jobs` library manages the scheduled jobs of a space. Job definitions are stored on the space and pushed to its agent, so they survive restarts and can be changed while the space is stopped.

Functions that change definitions (`add`, `update`, `remove`, `enable`, `disable`, `enable_runner`, `disable_runner`) require the **Edit Space Jobs** permission; `list` needs only **Use Spaces**, and `run` requires the **Run Commands** permission. All of them work on spaces you own or that are shared with you.

`run` is not available inside AI scripts (`run_script`), which may only call `/api/` endpoints; it raises an error there.

---

## Functions

| Function | Description |
|----------|-------------|
| `list(space)` | List a space's job definitions and runner state |
| `run(space, name)` | Trigger a job immediately by name |
| `add(space, name, command, schedule="", enabled=True)` | Add a job to a space |
| `update(space, name, command=None, schedule=None, enabled=None)` | Update a job's command, schedule or enabled state |
| `remove(space, name)` | Remove a job from a space |
| `enable(space, name)` | Enable a job so it fires automatically |
| `disable(space, name)` | Disable a job so it does not fire automatically |
| `enable_runner(space)` | Start the space's job runner |
| `disable_runner(space)` | Stop the space's job runner (manual runs still work) |

---

## Usage

```python
import knot.apiclient
import knot.jobs

knot.apiclient.configure("https://knot.example.com", "your-token")

# List jobs (works while the space is stopped)
jobs = knot.jobs.list("my-space")
print(jobs["enabled"])
for job in jobs["jobs"]:
    print(job["name"], job.get("schedule") or "(manual only)", job["enabled"])

# Add a scheduled job (daily at 02:00)
knot.jobs.add("my-space", "backup", command="./backup.sh", schedule="0 2 * * *")

# Add a manual-only job
knot.jobs.add("my-space", "cleanup", command="./clean.sh")

# Update the schedule, make it manual only with schedule=""
knot.jobs.update("my-space", "backup", schedule="*/30 * * * *")

# Pause one job, run it anyway, re-enable it
knot.jobs.disable("my-space", "backup")
knot.jobs.run("my-space", "backup")
knot.jobs.enable("my-space", "backup")

# Stop all scheduled firing (manual runs keep working), then restart it
knot.jobs.disable_runner("my-space")
knot.jobs.enable_runner("my-space")

# Remove a job
knot.jobs.remove("my-space", "backup")
```

---

## Function Reference

### list

```python
list(space)
```

List a space's job definitions and runner state. Works while the space is stopped.

**Parameters:**
- `space` - Space name or ID

**Returns:** A dict with `jobs` (list of job definition dicts) and `enabled` (bool, the runner state).

### run

```python
run(space, name)
```

Trigger a job immediately by name. Works for disabled and manual-only jobs; the space must be running. Requires the **Run Commands** permission. Raises an `Exception` (carrying the server's error, e.g. when the space is not running or the job does not exist) if the job could not be started.

**Parameters:**
- `space` - Space name or ID
- `name` - The name of the job to run

**Returns:** `True` once the job has started.

### add

```python
add(space, name, command, schedule="", enabled=True)
```

Add a job to a space. Raises `ValueError` if a job with the same name already exists.

**Parameters:**
- `space` - Space name or ID
- `name` - Job name, unique within the space
- `command` - Shell command the job runs in the space
- `schedule` - 5-field cron expression (`minute hour day month weekday`), e.g. `"0 2 * * *"` or `"*/5 * * * *"`; empty for a manual-only job
- `enabled` - If `False` the job is listed but never fires automatically

**Returns:** `True` on success.

### update

```python
update(space, name, command=None, schedule=None, enabled=None)
```

Update a job's command, schedule or enabled state. Only the given arguments are changed; `None` leaves a field unchanged. Pass `schedule=""` to make the job manual only. Raises `ValueError` if the job does not exist.

**Parameters:**
- `space` - Space name or ID
- `name` - The name of the job to update
- `command` - New shell command, or `None` to keep
- `schedule` - New cron expression, `""` for manual only, or `None` to keep
- `enabled` - New enabled state, or `None` to keep

**Returns:** `True` on success.

### remove

```python
remove(space, name)
```

Remove a job from a space and return `True`. Raises `ValueError` if the job does not exist.

### enable / disable

```python
enable(space, name)
disable(space, name)
```

Enable or disable a job and return `True`. A disabled job never fires automatically; manual triggering keeps working either way.

### enable_runner / disable_runner

```python
enable_runner(space)
disable_runner(space)
```

Start or stop the space's job runner and return `True`. A stopped runner suspends all scheduled firing while manual runs keep working. The state is persisted on the space.

---

## Job Properties

Each job definition contains:
- `name` - The job name, unique within the space
- `command` - The shell command the job runs in the space's home directory
- `schedule` - 5-field cron expression; **omitted** for a manual-only job, so read it with `job.get("schedule")`
- `enabled` - Whether the job fires automatically

---

## See Also

- [Space Jobs](https://getknot.dev/docs/spaces/jobs/) — the user-facing guide, including the cron schedule syntax and behaviour notes (no catch-up while stopped, overlap skipping, run history)
- [knot.template](https://getknot.dev/reference/libraries/template/) — templates can define jobs that are copied into new spaces (`create(..., jobs=[...])`)
