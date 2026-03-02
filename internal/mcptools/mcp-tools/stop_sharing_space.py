import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")

success = knot.space.unshare(space_name)

if success:
    tool.return_string(f"Space '{space_name}' unshared successfully")
else:
    tool.return_error("Failed to stop sharing space")
