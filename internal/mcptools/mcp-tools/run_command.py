import knot.space
import knot.mcp

space_name = knot.mcp.get("space_name")
command = knot.mcp.get("command")
args = knot.mcp.get("arguments", [])
timeout = knot.mcp.get("timeout", 30)
workdir = knot.mcp.get("workdir", "")

kwargs = {"args": args, "timeout": timeout}
if workdir:
    kwargs["workdir"] = workdir

output = knot.space.run(space_name, command, **kwargs)

knot.mcp.return_object({"output": output, "success": True})
