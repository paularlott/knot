import knot.space
import knot.mcp

space_name = knot.mcp.get_string("name")

success = knot.space.unshare(space_name)

if success:
    knot.mcp.return_string(f"Space '{space_name}' unshared successfully")
else:
    knot.mcp.return_error("Failed to stop sharing space")
