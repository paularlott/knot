import knot.pool
import scriptling.mcp.tool as tool

space = tool.get_string("space")
time = tool.get_string("time", "")

try:
    tool.return_object(knot.pool.extend(space, time=time or None))
except Exception as e:
    tool.return_error(str(e))
