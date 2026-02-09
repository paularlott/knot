import knot.space
import knot.mcp

space_name = knot.mcp.get_string("name")
success = knot.space.restart(space_name)

if success:
    knot.mcp.return_string(f"Space '{space_name}' restarted successfully")
else:
    knot.mcp.return_error("Failed to restart space")
