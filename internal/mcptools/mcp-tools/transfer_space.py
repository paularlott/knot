import knot.space
import knot.mcp

space_name = knot.mcp.get_string("name")
user_id = knot.mcp.get_string("user_id")

success = knot.space.transfer(space_name, user_id)

if success:
   knot.mcp.return_string(f"Space '{space_name}' transferred successfully to user '{user_id}'")
else:
    knot.mcp.return_error("Failed to transfer space")
