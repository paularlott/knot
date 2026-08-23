# knot.jobs - Space jobs library for Knot server
#
# This library provides functions for managing the scheduled jobs of a space.
# Job definitions are stored on the space and pushed to its agent, so they
# survive restarts and can be changed while the space is stopped.
# Requires knot.apiclient to be configured first.
#
# Usage:
#   import knot.apiclient
#   import knot.jobs
#
#   knot.apiclient.configure("https://knot.example.com", "your-token")
#   knot.jobs.add("my-space", "backup", command="./backup.sh", schedule="0 2 * * *")
#   knot.jobs.run("my-space", "backup")
#   jobs = knot.jobs.list("my-space")

import knot.apiclient as api
import urllib.parse


def _enc(s):
    """URL-encode a path segment for safe interpolation into a URL."""
    return urllib.parse.quote(str(s), safe='')


def list(space):
    """List the jobs defined on a space.

    Returns a dict with 'jobs' (the definitions) and 'enabled' (the runner
    state). Works while the space is stopped.
    """
    return api.get(f"/api/spaces/{_enc(space)}/jobs")


def run(space, name):
    """Trigger a job immediately by name.

    Works for disabled and manual-only jobs. The space must be running.
    Returns True if the job was started.
    """
    response = api.post(f"/space-io/{_enc(space)}/jobs/run", {"name": name})
    if not response.get("success", False):
        raise RuntimeError(response.get("error", f"failed to run job '{name}'"))
    return True


def add(space, name, command, schedule="", enabled=True):
    """Add a job to a space.

    Args:
        space: Space name or ID.
        name: Job name, unique within the space.
        command: Shell command the job runs in the space.
        schedule: 5-field cron expression, e.g. "0 2 * * *" or "*/5 * * * *".
            Empty for a manual-only job.
        enabled: If False the job is listed but never fires automatically.
    """
    def mutate(jobs):
        jobs.append({
            "name": name,
            "command": command,
            "schedule": schedule,
            "enabled": enabled,
        })

    return _mutate(space, mutate, exists_error=name)


def update(space, name, command=None, schedule=None, enabled=None):
    """Update a job's command, schedule or enabled state.

    Only the given arguments are changed; None leaves a field unchanged.
    Pass schedule="" to make the job manual only.
    """
    def mutate(jobs):
        for job in jobs:
            if job.get("name") == name:
                if command is not None:
                    job["command"] = command
                if schedule is not None:
                    job["schedule"] = schedule
                if enabled is not None:
                    job["enabled"] = enabled
                return
        raise ValueError(f"job '{name}' not found")

    return _mutate(space, mutate)


def remove(space, name):
    """Remove a job from a space."""
    def mutate(jobs):
        kept = [job for job in jobs if job.get("name") != name]
        if len(kept) == len(jobs):
            raise ValueError(f"job '{name}' not found")
        jobs[:] = kept

    return _mutate(space, mutate)


def enable(space, name):
    """Enable a job so it fires automatically."""
    return update(space, name, enabled=True)


def disable(space, name):
    """Disable a job so it does not fire automatically. Manual runs still work."""
    return update(space, name, enabled=False)


def enable_runner(space):
    """Start the space's job runner: scheduled jobs fire."""
    return _set_runner(space, True)


def disable_runner(space):
    """Stop the space's job runner. Manual runs still work."""
    return _set_runner(space, False)


def _set_runner(space, enabled):
    current = api.get(f"/api/spaces/{_enc(space)}/jobs")
    api.put(f"/api/spaces/{_enc(space)}/jobs", {
        "jobs": current.get("jobs", []),
        "enabled": enabled,
    })
    return True


def _mutate(space, mutate, exists_error=None):
    """Fetch the space's definitions, apply mutate to the job list, save."""
    current = api.get(f"/api/spaces/{_enc(space)}/jobs")
    jobs = current.get("jobs", [])

    if exists_error is not None:
        for job in jobs:
            if job.get("name") == exists_error:
                raise ValueError(f"job '{exists_error}' already exists")

    mutate(jobs)
    api.put(f"/api/spaces/{_enc(space)}/jobs", {
        "jobs": jobs,
        "enabled": current.get("enabled", True),
    })
    return True
