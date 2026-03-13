import knot.template
import scriptling.mcp.tool as tool

template_id = tool.get_string("template_id")

# Build kwargs with only provided values
kwargs = {}

name = tool.get_string("name", "")
if name:
    kwargs["name"] = name

job = tool.get_string("job", "")
if job:
    kwargs["job"] = job

description = tool.get_string("description", "")
if description:
    kwargs["description"] = description

platform = tool.get_string("platform", "")
if platform:
    kwargs["platform"] = platform

volumes = tool.get_string("volumes", "")
if volumes:
    kwargs["volumes"] = volumes

# For boolean/numeric params, we need a way to detect if they were provided
# Using a sentinel pattern - check if the param exists in the raw arguments
active = tool.get_bool("active", None)
if active is not None:
    kwargs["active"] = active

compute_units = tool.get_int("compute_units", -1)
if compute_units >= 0:
    kwargs["compute_units"] = compute_units

storage_units = tool.get_int("storage_units", -1)
if storage_units >= 0:
    kwargs["storage_units"] = storage_units

with_terminal = tool.get_bool("with_terminal", None)
if with_terminal is not None:
    kwargs["with_terminal"] = with_terminal

with_vscode_tunnel = tool.get_bool("with_vscode_tunnel", None)
if with_vscode_tunnel is not None:
    kwargs["with_vscode_tunnel"] = with_vscode_tunnel

with_code_server = tool.get_bool("with_code_server", None)
if with_code_server is not None:
    kwargs["with_code_server"] = with_code_server

with_ssh = tool.get_bool("with_ssh", None)
if with_ssh is not None:
    kwargs["with_ssh"] = with_ssh

with_run_command = tool.get_bool("with_run_command", None)
if with_run_command is not None:
    kwargs["with_run_command"] = with_run_command

schedule_enabled = tool.get_bool("schedule_enabled", None)
if schedule_enabled is not None:
    kwargs["schedule_enabled"] = schedule_enabled

icon_url = tool.get_string("icon_url", "")
if icon_url:
    kwargs["icon_url"] = icon_url

groups = tool.get_string_list("groups", [])
if groups:
    kwargs["groups"] = groups

zones = tool.get_string_list("zones", [])
if zones:
    kwargs["zones"] = zones

if not kwargs:
    tool.return_error("No update parameters provided")

success = knot.template.update(template_id, **kwargs)

if success:
    tool.return_string(f"Template '{template_id}' updated successfully")
else:
    tool.return_error(f"Failed to update template '{template_id}'")
