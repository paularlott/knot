import knot.template
import scriptling.mcp.tool as tool

name = tool.get_string("name")
job = tool.get_string("job", "")
description = tool.get_string("description", "")
platform = tool.get_string("platform", "")
volumes = tool.get_string("volumes", "")
active = tool.get_bool("active", True)
compute_units = tool.get_int("compute_units", 0)
storage_units = tool.get_int("storage_units", 0)
with_terminal = tool.get_bool("with_terminal", False)
with_vscode_tunnel = tool.get_bool("with_vscode_tunnel", False)
with_code_server = tool.get_bool("with_code_server", False)
with_ssh = tool.get_bool("with_ssh", False)
with_run_command = tool.get_bool("with_run_command", False)
schedule_enabled = tool.get_bool("schedule_enabled", False)
icon_url = tool.get_string("icon_url", "")
groups = tool.get_string_list("groups", [])
zones = tool.get_string_list("zones", [])

template_id = knot.template.create(
    name=name,
    job=job,
    description=description,
    platform=platform,
    volumes=volumes,
    active=active,
    compute_units=compute_units,
    storage_units=storage_units,
    with_terminal=with_terminal,
    with_vscode_tunnel=with_vscode_tunnel,
    with_code_server=with_code_server,
    with_ssh=with_ssh,
    with_run_command=with_run_command,
    schedule_enabled=schedule_enabled,
    icon_url=icon_url,
    groups=groups if groups else None,
    zones=zones if zones else None
)

tool.return_string(f"Template '{name}' created successfully with ID: {template_id}")
