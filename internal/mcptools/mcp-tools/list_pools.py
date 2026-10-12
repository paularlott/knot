import knot.pool
import scriptling.mcp.tool as tool

name = tool.get_string("name", "")

try:
    if name:
        tool.return_object(knot.pool.get(name))
    else:
        tool.return_object({"pools": knot.pool.list()})
except Exception as e:
    tool.return_error(str(e))
