import knot.space
import knot.mcp

space_name = knot.mcp.get_string("name")

# Check if space is already running
if knot.space.is_running(space_name):
    knot.mcp.return_error("Space is already running")
else:
    success = knot.space.start(space_name)

    if success:
        knot.mcp.return_string(f"Space '{space_name}' started successfully")
    else:
        knot.mcp.return_error("Failed to start space")
