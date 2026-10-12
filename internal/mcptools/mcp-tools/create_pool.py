import knot.pool
import scriptling.mcp.tool as tool

name = tool.get_string("name")
template_name = tool.get_string("template_name")
desired_count = tool.get_int("desired_count", 1)
startup_script_name = tool.get_string("startup_script_name", "")
active = tool.get_bool("active", False)
lease_max_time = tool.get_int("lease_max_time", 0)
lease_max_extensions = tool.get_int("lease_max_extensions", 0)

custom_fields = []
for custom_field in tool.get_string_list("custom_fields", []):
    if "=" not in custom_field:
        tool.return_error("custom_fields entries must use name=value format (e.g. \"ENV=dev\")")
    field_name, field_value = custom_field.split("=", 1)
    if not field_name.strip():
        tool.return_error("custom_fields entries must include a field name")
    custom_fields.append({"name": field_name.strip(), "value": field_value})

startup_script_id = ""
if startup_script_name:
    import knot.script
    script = knot.script.get(startup_script_name)
    if script:
        startup_script_id = script.get("id", "")
    if not startup_script_id:
        tool.return_error(f"Startup script not found: {startup_script_name}")

pool_id = knot.pool.create(
    name,
    template_name,
    startup_script_id=startup_script_id,
    desired_count=desired_count,
    active=active,
    lease_max_time=lease_max_time,
    lease_max_extensions=lease_max_extensions,
    custom_fields=custom_fields,
)

state = "started" if active else "created stopped"
tool.return_string(f"Pool '{name}' {state} with {desired_count} space(s), ID: {pool_id}")
