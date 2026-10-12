import knot.apiclient as api
import scriptling.mcp.tool as tool
import urllib.parse

space_name = tool.get_string("name")
command = tool.get_string("command")
args = tool.get_list("arguments", [])
timeout = tool.get_int("timeout", 30)
workdir = tool.get_string("workdir", "")

response = api.post(f"/api/spaces/{urllib.parse.quote(space_name, safe='')}/run-command", {
    "command": command,
    "args": args,
    "timeout": timeout,
    "workdir": workdir,
})

# A failed command still returns its output, which usually says why
result = {
    "output": response.get("output", ""),
    "success": response.get("success", False),
    "exit_code": response.get("exit_code", 0),
}
if not result["success"]:
    result["error"] = response.get("error", "")
tool.return_object(result)
