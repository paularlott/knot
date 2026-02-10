import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
success = knot.space.restart(space_name)

if success:
    tool.return_string(f"Space '{space_name}' restarted successfully")
else:
    tool.return_error("Failed to restart space")
