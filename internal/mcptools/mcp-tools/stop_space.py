import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")

# Check if space is already stopped
if not knot.space.is_running(space_name):
    tool.return_error("Space is already stopped")
else:
    success = knot.space.stop(space_name)

    if success:
        tool.return_string(f"Space '{space_name}' stopped successfully")
    else:
        tool.return_error("Failed to stop space")
