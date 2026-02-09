import knot.space
import knot.mcp

space_name = knot.mcp.get_string("name")
user_id = knot.mcp.get_string("user_id")

success = knot.space.share(space_name, user_id)

if success:
    knot.mcp.return_string(f"Space '{space_name}' shared successfully with user '{user_id}'")
else:
    knot.mcp.return_error("Failed to share space")
