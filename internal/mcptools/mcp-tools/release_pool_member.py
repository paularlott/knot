import knot.pool
import scriptling.mcp.tool as tool

space = tool.get_string("space")
destroy = tool.get_bool("destroy", False)

try:
    tool.return_object(knot.pool.release(space, destroy=destroy))
except Exception as e:
    tool.return_error(str(e))
