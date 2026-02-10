import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
user_id = tool.get_string("user_id")

success = knot.space.transfer(space_name, user_id)

if success:
   tool.return_string(f"Space '{space_name}' transferred successfully to user '{user_id}'")
else:
    tool.return_error("Failed to transfer space")
