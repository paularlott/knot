# knot.template - Template management library for Knot server

from . import api

def list():
    """List all templates."""
    response = api.get("/api/templates")

    result = []
    for tmpl in response.get("templates", []):
        result.append({
            "id": tmpl.get("template_id"),
            "name": tmpl.get("name"),
            "description": tmpl.get("description", ""),
            "platform": tmpl.get("platform", ""),
            "active": tmpl.get("active", False),
            "usage": tmpl.get("usage", 0),
            "deployed": tmpl.get("deployed", 0)
        })

    return result


def get(template_id):
    """Get template by ID or name."""
    response = api.get(f"/api/templates/{template_id}")
    return _parse_template(response)


def create(name, job="", description="", platform="", volumes="", active=True,
           compute_units=0, storage_units=0, with_terminal=False,
           with_vscode_tunnel=False, with_code_server=False, with_ssh=False,
           with_run_command=False, schedule_enabled=False, icon_url="",
           groups=None, zones=None):
    """Create a new template."""
    body = {
        "name": name,
        "job": job,
        "description": description,
        "platform": platform,
        "volumes": volumes,
        "active": active,
        "compute_units": compute_units,
        "storage_units": storage_units,
        "with_terminal": with_terminal,
        "with_vscode_tunnel": with_vscode_tunnel,
        "with_code_server": with_code_server,
        "with_ssh": with_ssh,
        "with_run_command": with_run_command,
        "schedule_enabled": schedule_enabled,
        "icon_url": icon_url,
        "groups": groups or [],
        "zones": zones or [],
        "schedule": [],
        "custom_fields": []
    }

    response = api.post("/api/templates", body)
    return response.get("template_id")


def update(template_id, name=None, job=None, description=None, platform=None,
           volumes=None, active=None, compute_units=None, storage_units=None,
           with_terminal=None, with_vscode_tunnel=None, with_code_server=None,
           with_ssh=None, with_run_command=None, schedule_enabled=None,
           icon_url=None, groups=None, zones=None):
    """Update template properties."""
    current = api.get(f"/api/templates/{template_id}")

    body = {
        "name": name if name is not None else current.get("name"),
        "job": job if job is not None else current.get("job", ""),
        "description": description if description is not None else current.get("description", ""),
        "platform": platform if platform is not None else current.get("platform", ""),
        "volumes": volumes if volumes is not None else current.get("volumes", ""),
        "active": active if active is not None else current.get("active", True),
        "compute_units": compute_units if compute_units is not None else current.get("compute_units", 0),
        "storage_units": storage_units if storage_units is not None else current.get("storage_units", 0),
        "with_terminal": with_terminal if with_terminal is not None else current.get("with_terminal", False),
        "with_vscode_tunnel": with_vscode_tunnel if with_vscode_tunnel is not None else current.get("with_vscode_tunnel", False),
        "with_code_server": with_code_server if with_code_server is not None else current.get("with_code_server", False),
        "with_ssh": with_ssh if with_ssh is not None else current.get("with_ssh", False),
        "with_run_command": with_run_command if with_run_command is not None else current.get("with_run_command", False),
        "schedule_enabled": schedule_enabled if schedule_enabled is not None else current.get("schedule_enabled", False),
        "icon_url": icon_url if icon_url is not None else current.get("icon_url", ""),
        "groups": groups if groups is not None else current.get("groups", []),
        "zones": zones if zones is not None else current.get("zones", []),
        "schedule": current.get("schedule", []),
        "custom_fields": current.get("custom_fields", []),
        "startup_script_id": current.get("startup_script_id", ""),
        "shutdown_script_id": current.get("shutdown_script_id", ""),
        "auto_start": current.get("auto_start", False),
        "max_uptime": current.get("max_uptime", 0),
        "max_uptime_unit": current.get("max_uptime_unit", "hours")
    }

    api.put(f"/api/templates/{template_id}", body)
    return True


def delete(template_id):
    """Delete a template."""
    api.delete(f"/api/templates/{template_id}")
    return True


def get_icons():
    """Get list of available icons."""
    response = api.get("/api/icons")

    result = []
    for icon in response:
        result.append({
            "description": icon.get("description", ""),
            "source": icon.get("source", ""),
            "url": icon.get("url", "")
        })

    return result


def _parse_template(response):
    """Parse a template response into a standardized dict."""
    schedule = []
    for day in response.get("schedule", []):
        schedule.append({
            "enabled": day.get("enabled", False),
            "from": day.get("from", ""),
            "to": day.get("to", "")
        })

    custom_fields = []
    for cf in response.get("custom_fields", []):
        custom_fields.append({
            "name": cf.get("name", ""),
            "description": cf.get("description", "")
        })

    return {
        "id": response.get("template_id"),
        "name": response.get("name"),
        "description": response.get("description", ""),
        "platform": response.get("platform", ""),
        "job": response.get("job", ""),
        "volumes": response.get("volumes", ""),
        "active": response.get("active", False),
        "is_managed": response.get("is_managed", False),
        "compute_units": response.get("compute_units", 0),
        "storage_units": response.get("storage_units", 0),
        "usage": response.get("usage", 0),
        "deployed": response.get("deployed", 0),
        "hash": response.get("hash", ""),
        "with_terminal": response.get("with_terminal", False),
        "with_vscode_tunnel": response.get("with_vscode_tunnel", False),
        "with_code_server": response.get("with_code_server", False),
        "with_ssh": response.get("with_ssh", False),
        "with_run_command": response.get("with_run_command", False),
        "schedule_enabled": response.get("schedule_enabled", False),
        "auto_start": response.get("auto_start", False),
        "max_uptime": response.get("max_uptime", 0),
        "max_uptime_unit": response.get("max_uptime_unit", "hours"),
        "icon_url": response.get("icon_url", ""),
        "groups": response.get("groups", []),
        "zones": response.get("zones", []),
        "schedule": schedule,
        "custom_fields": custom_fields
    }
