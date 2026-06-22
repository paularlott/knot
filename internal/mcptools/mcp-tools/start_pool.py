import knot.pool
import scriptling.mcp.tool as tool

name = tool.get_string("name")

try:
    knot.pool.start(name)
    tool.return_string(f"Pool '{name}' started")
except Exception as e:
    tool.return_error(str(e))
