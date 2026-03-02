import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
script_name = tool.get_string("script")
args = tool.get_list("arguments", [])

result = knot.space.run_script(space_name, script_name, *args)

tool.return_object({
    "output": result["output"],
    "exit_code": result["exit_code"],
    "success": result["exit_code"] == 0,
})
