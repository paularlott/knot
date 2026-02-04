import knot.space
import knot.mcp

space_name = knot.mcp.get_string("name")
command = knot.mcp.get_string("command")
args = knot.mcp.get_list("arguments", [])
timeout = knot.mcp.get_int("timeout", 30)
workdir = knot.mcp.get_string("workdir", "")

kwargs = {"args": args, "timeout": timeout}
if workdir:
    kwargs["workdir"] = workdir

output = knot.space.run(space_name, command, **kwargs)

knot.mcp.return_object({"output": output, "success": True})
