import knot.space
import knot.mcp

space_name = knot.mcp.get("name")

# Check if space is already running
if knot.space.is_running(space_name):
    knot.mcp.return_error("Space is already running")
else:
    success = knot.space.start(space_name)

    if success:
        knot.mcp.return_object({"status": True})
    else:
        knot.mcp.return_error("Failed to start space")
