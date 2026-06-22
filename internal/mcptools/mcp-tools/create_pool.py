import knot.pool
import scriptling.mcp.tool as tool

name = tool.get_string("name")
template_name = tool.get_string("template_name")
desired_count = tool.get_int("desired_count", 1)
startup_script_name = tool.get_string("startup_script_name", "")
active = tool.get_bool("active", False)

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
)

state = "started" if active else "created stopped"
tool.return_string(f"Pool '{name}' {state} with {desired_count} space(s), ID: {pool_id}")
