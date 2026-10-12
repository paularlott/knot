import knot.pool
import scriptling.mcp.tool as tool

name = tool.get_string("name")

try:
    tool.return_object({"leases": knot.pool.leases(name)})
except Exception as e:
    tool.return_error(str(e))
