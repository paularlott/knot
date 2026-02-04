import knot.space
import knot.mcp

space_name = knot.mcp.get_string("name")
success = knot.space.delete(space_name)

if success:
    knot.mcp.return_string(f"Space '{space_name}' deleted successfully")
else:
    knot.mcp.return_error("Failed to delete space")
