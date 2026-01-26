import knot.space
import knot.mcp

space_name = knot.mcp.get("space_name")
user_id = knot.mcp.get("user_id")

success = knot.space.share(space_name, user_id)

if success:
    knot.mcp.return_object({"status": True})
else:
    knot.mcp.return_error("Failed to share space")
