import knot.pool
import scriptling.mcp.tool as tool

name = tool.get_string("name")
time = tool.get_string("time", "")
wait = tool.get_string("wait", "")

try:
    lease = knot.pool.acquire(name, time=time or None, wait=wait or None)
    tool.return_object(lease)
except Exception as e:
    tool.return_error(str(e))
