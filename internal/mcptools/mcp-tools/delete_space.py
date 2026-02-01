import knot.space
import knot.mcp

space_name = knot.mcp.get("name")
success = knot.space.delete(space_name)

if success:
    knot.mcp.return_object({"status": True})
else:
    knot.mcp.return_error("Failed to delete space")
