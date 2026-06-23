import knot.pool
import scriptling.mcp.tool as tool

name = tool.get_string("name")
desired_count = tool.get_int("desired_count")

try:
    knot.pool.set_size(name, desired_count)
    tool.return_string(f"Pool '{name}' size set to {desired_count}")
except Exception as e:
    tool.return_error(str(e))
