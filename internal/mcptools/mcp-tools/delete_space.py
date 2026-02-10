import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
success = knot.space.delete(space_name)

if success:
    tool.return_string(f"Space '{space_name}' deleted successfully")
else:
    tool.return_error("Failed to delete space")
