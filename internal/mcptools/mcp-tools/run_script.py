import knot.space
import knot.mcp

space_name = knot.mcp.get_string("name")
script_name = knot.mcp.get_string("script")
args = knot.mcp.get_list("arguments", [])

result = knot.space.run_script(space_name, script_name, *args)

knot.mcp.return_object({
    "output": result["output"],
    "exit_code": result["exit_code"],
    "success": result["exit_code"] == 0,
})
