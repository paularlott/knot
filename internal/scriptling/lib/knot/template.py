# knot.template - Template management library for Knot server

import json

import knot.apiclient as api

def list(include_inactive=False):
    """List all templates visible to the current user.

    Args:
        include_inactive: If True, include inactive templates. Default False
            (only active templates are returned).
    """
    response = api.get("/api/templates")

    result = []
    for tmpl in response.get("templates", []):
        if not include_inactive and not tmpl.get("active", False):
            continue

        custom_fields = []
        for cf in tmpl.get("custom_fields", []):
            custom_fields.append({
                "name": cf.get("name", ""),
                "description": cf.get("description", ""),
            })

        result.append({
            "id": tmpl.get("template_id"),
            "name": tmpl.get("name"),
            "description": tmpl.get("description", ""),
            "platform": tmpl.get("platform", ""),
            "active": tmpl.get("active", False),
            "usage": tmpl.get("usage", 0),
            "deployed": tmpl.get("deployed", 0),
            "custom_fields": custom_fields,
        })

    return result


def get(template_id):
    """Get template by ID or name."""
    response = api.get(f"/api/templates/{template_id}")
    return _parse_template(response)


def validate(platform, job="", volumes=""):
    """Validate template job and volume specifications without saving."""
    return api.post("/api/templates/validate", {
        "platform": platform,
        "job": job,
        "volumes": volumes,
    })


def nodes(template_id):
    """List available nodes for a local-container template."""
    response = api.get(f"/api/templates/{template_id}/nodes")
    return [{
        "node_id": n.get("node_id", ""),
        "hostname": n.get("hostname", ""),
        "running_spaces": n.get("running_spaces", 0),
        "total_spaces": n.get("total_spaces", 0),
    } for n in response]


def create(name, job="", description="", platform="", volumes="", active=True,
           compute_units=0, storage_units=0, with_terminal=False,
           with_vscode_tunnel=False, with_code_server=False, with_ssh=False,
           with_run_command=False, allow_node_migration=False,
           schedule_enabled=False, icon_url="",
           groups=None, zones=None, paths=None, disable_user_activity=False,
           health_check_type="none", health_check_config="", health_check_skip_ssl_verify=False,
           health_check_timeout=10, health_check_interval=30, health_check_max_failures=3,
           health_check_auto_restart=False, ports=None):
    """Create a new template."""
    volumes = _with_paths(volumes, paths)
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
        "allow_node_migration": allow_node_migration,
        "schedule_enabled": schedule_enabled,
        "icon_url": icon_url,
        "groups": groups or [],
        "zones": zones or [],
        "schedule": [],
        "custom_fields": [],
        "disable_user_activity": disable_user_activity,
        "health_check_type": health_check_type,
        "health_check_config": "" if health_check_type in ("none", "agent") else health_check_config,
        "health_check_skip_ssl_verify": health_check_skip_ssl_verify,
        "health_check_timeout": health_check_timeout,
        "health_check_interval": health_check_interval,
        "health_check_max_failures": health_check_max_failures,
        "health_check_auto_restart": health_check_auto_restart,
        "ports": ports or [],
    }

    response = api.post("/api/templates", body)
    return response.get("template_id")


def update(template_id, name=None, job=None, description=None, platform=None,
           volumes=None, active=None, compute_units=None, storage_units=None,
           with_terminal=None, with_vscode_tunnel=None, with_code_server=None,
           with_ssh=None, with_run_command=None, allow_node_migration=None,
           schedule_enabled=None,
           icon_url=None, groups=None, zones=None, paths=None, disable_user_activity=None,
           health_check_type=None, health_check_config=None, health_check_skip_ssl_verify=None,
           health_check_timeout=None, health_check_interval=None, health_check_max_failures=None,
           health_check_auto_restart=None, ports=None):
    """Update template properties."""
    current = api.get(f"/api/templates/{template_id}")
    volumes_value = volumes if volumes is not None else current.get("volumes", "")
    volumes_value = _with_paths(volumes_value, paths)

    body = {
        "name": name if name is not None else current.get("name"),
        "job": job if job is not None else current.get("job", ""),
        "description": description if description is not None else current.get("description", ""),
        "platform": platform if platform is not None else current.get("platform", ""),
        "volumes": volumes_value,
        "active": active if active is not None else current.get("active", True),
        "compute_units": compute_units if compute_units is not None else current.get("compute_units", 0),
        "storage_units": storage_units if storage_units is not None else current.get("storage_units", 0),
        "with_terminal": with_terminal if with_terminal is not None else current.get("with_terminal", False),
        "with_vscode_tunnel": with_vscode_tunnel if with_vscode_tunnel is not None else current.get("with_vscode_tunnel", False),
        "with_code_server": with_code_server if with_code_server is not None else current.get("with_code_server", False),
        "with_ssh": with_ssh if with_ssh is not None else current.get("with_ssh", False),
        "with_run_command": with_run_command if with_run_command is not None else current.get("with_run_command", False),
        "allow_node_migration": allow_node_migration if allow_node_migration is not None else current.get("allow_node_migration", False),
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
        "max_uptime_unit": current.get("max_uptime_unit", "hours"),
        "disable_user_activity": disable_user_activity if disable_user_activity is not None else current.get("disable_user_activity", False),
        "health_check_type": health_check_type if health_check_type is not None else current.get("health_check_type", "none"),
        "health_check_config": health_check_config if health_check_config is not None else current.get("health_check_config", ""),
        "health_check_skip_ssl_verify": health_check_skip_ssl_verify if health_check_skip_ssl_verify is not None else current.get("health_check_skip_ssl_verify", False),
        "health_check_timeout": health_check_timeout if health_check_timeout is not None else current.get("health_check_timeout", 10),
        "health_check_interval": health_check_interval if health_check_interval is not None else current.get("health_check_interval", 30),
        "health_check_max_failures": health_check_max_failures if health_check_max_failures is not None else current.get("health_check_max_failures", 3),
        "health_check_auto_restart": health_check_auto_restart if health_check_auto_restart is not None else current.get("health_check_auto_restart", False),
        "ports": ports if ports is not None else current.get("ports", []),
    }
    if body["health_check_type"] in ("none", "agent"):
        body["health_check_config"] = ""

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


def _strip_paths_block(yaml_str):
    """Remove a top-level 'paths:' block from a YAML string."""
    lines = yaml_str.splitlines(keepends=True)
    out = []
    in_paths = False
    for line in lines:
        if in_paths:
            if line.startswith(' ') or line.startswith('\t') or not line.strip():
                continue
            in_paths = False
        if line.rstrip('\r\n') == 'paths:':
            in_paths = True
        else:
            out.append(line)
    return ''.join(out)


def _with_paths(volumes, paths):
    """Append managed path definitions to a template volume specification."""
    if paths is None:
        return volumes

    if isinstance(paths, str):
        paths = [paths]

    paths = [path for path in paths if path]
    if not paths:
        return volumes

    result = _strip_paths_block(volumes or "").rstrip()
    if result:
        result += "\n"

    result += "paths:\n"
    for path in paths:
        result += f"  - {json.dumps(path)}\n"

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
        "allow_node_migration": response.get("allow_node_migration", False),
        "schedule_enabled": response.get("schedule_enabled", False),
        "auto_start": response.get("auto_start", False),
        "max_uptime": response.get("max_uptime", 0),
        "max_uptime_unit": response.get("max_uptime_unit", "hours"),
        "icon_url": response.get("icon_url", ""),
        "groups": response.get("groups", []),
        "zones": response.get("zones", []),
        "schedule": schedule,
        "custom_fields": custom_fields,
        "disable_user_activity": response.get("disable_user_activity", False),
        "health_check_type": response.get("health_check_type", "none"),
        "health_check_config": response.get("health_check_config", ""),
        "health_check_skip_ssl_verify": response.get("health_check_skip_ssl_verify", False),
        "health_check_timeout": response.get("health_check_timeout", 10),
        "health_check_interval": response.get("health_check_interval", 30),
        "health_check_max_failures": response.get("health_check_max_failures", 3),
        "health_check_auto_restart": response.get("health_check_auto_restart", False),
        "ports": response.get("ports", []),
    }
