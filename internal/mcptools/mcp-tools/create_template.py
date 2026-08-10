import knot.template
import scriptling.mcp.tool as tool

name = tool.get_string("name")
image = tool.get_string("image")
description = tool.get_string("description", "")
platform = tool.get_string("platform", "nomad")
hostname = tool.get_string("hostname", "")
memory = tool.get_string("memory", "")
cpus = tool.get_string("cpus", "")
command = tool.get_string_list("command", [])
environment_args = tool.get_string_list("environment", [])
port_args = tool.get_string_list("ports", [])
groups = tool.get_string_list("groups", [])
zones = tool.get_string_list("zones", [])

with_terminal = tool.get_bool("with_terminal", False)
with_ssh = tool.get_bool("with_ssh", False)
with_code_server = tool.get_bool("with_code_server", False)
with_vscode_tunnel = tool.get_bool("with_vscode_tunnel", False)
with_run_command = tool.get_bool("with_run_command", False)
active = tool.get_bool("active", True)
compute_units = tool.get_int("compute_units", 0)
storage_units = tool.get_int("storage_units", 0)

if not name:
    tool.return_error("name is required")

environment = []
for env in environment_args:
    if isinstance(env, dict):
        key = env.get("key", env.get("name"))
        value = env.get("value")
        if key is None or not str(key).strip():
            tool.return_error("environment entries must include a key/name")
        environment.append({"key": str(key).strip(), "value": str(value) if value is not None else ""})
    elif isinstance(env, str):
        if "=" not in env:
            tool.return_error("environment entries must use KEY=value format (e.g. \"NODE_ENV=production\"), {\"key\": ..., \"value\": ...}, or {\"NODE_ENV\": \"production\"}")
        key, value = env.split("=", 1)
        if not key.strip():
            tool.return_error("environment entries must include a key")
        environment.append({"key": key.strip(), "value": value})
    else:
        tool.return_error("environment entries must be strings or dicts")

ports = []
for port in port_args:
    if isinstance(port, dict):
        port_name = str(port.get("name", "")).strip()
        protocol = str(port.get("protocol", "")).strip().lower() or "tcp"
        port_number = port.get("port")
    elif isinstance(port, str):
        protocol = "tcp"
        port_name = ""
        rest = port.strip()
        if "/" in rest:
            rest, protocol = rest.rsplit("/", 1)
            protocol = protocol.strip().lower() or "tcp"
        if ":" in rest:
            port_name, rest = rest.split(":", 1)
            port_name = port_name.strip()
            rest = rest.strip()
        port_number = rest
    else:
        tool.return_error("ports entries must be strings or dicts")

    try:
        port_number = int(port_number)
    except (TypeError, ValueError):
        tool.return_error("ports entries must include a numeric port (got: %r)" % (port,))

    if not (1 <= port_number <= 65535):
        tool.return_error("port number must be between 1 and 65535 (got: %d)" % port_number)
    if protocol not in ("http", "https", "tcp"):
        tool.return_error("port protocol must be http, https, or tcp (got: %s)" % protocol)

    ports.append({"name": port_name, "port": port_number, "protocol": protocol})

job = ""
volumes = ""
if platform != "manual":
    if not image:
        tool.return_error("image is required unless platform is 'manual'")

    spec = {"image": image}
    if hostname:
        spec["hostname"] = hostname
    if command:
        spec["command"] = list(command)
    if environment:
        spec["environment"] = environment
    if memory:
        spec["memory"] = memory
    if cpus:
        spec["cpus"] = cpus
        spec["cpu_type"] = "cores"

    try:
        built = knot.template.build_spec(platform, spec)
    except Exception as e:
        tool.return_error("failed to build template spec: %s" % e)

    job = built.get("job", "")
    volumes = built.get("volumes", "")

template_id = knot.template.create(
    name,
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
    groups=groups,
    zones=zones,
    ports=ports,
)

result = {
    "message": "Template '%s' created successfully" % name,
    "template_id": template_id,
    "platform": platform,
}
if image:
    result["image"] = image
if ports:
    result["ports"] = ports

tool.return_object(result)
