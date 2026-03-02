import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
command = tool.get_string("command")
args = tool.get_list("arguments", [])
timeout = tool.get_int("timeout", 30)
workdir = tool.get_string("workdir", "")

kwargs = {"args": args, "timeout": timeout}
if workdir:
    kwargs["workdir"] = workdir

output = knot.space.run(space_name, command, **kwargs)

tool.return_object({"output": output, "success": True})
