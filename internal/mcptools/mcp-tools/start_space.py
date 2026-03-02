import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")

# Check if space is already running
if knot.space.is_running(space_name):
    tool.return_error("Space is already running")
else:
    success = knot.space.start(space_name)

    if success:
        tool.return_string(f"Space '{space_name}' started successfully")
    else:
        tool.return_error("Failed to start space")
