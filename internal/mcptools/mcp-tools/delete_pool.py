import knot.pool
import scriptling.mcp.tool as tool

name = tool.get_string("name")

try:
    knot.pool.delete(name)
    tool.return_string(f"Pool '{name}' deleted")
except Exception as e:
    tool.return_error(str(e))
