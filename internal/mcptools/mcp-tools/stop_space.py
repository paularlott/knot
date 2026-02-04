import knot.space
import knot.mcp

space_name = knot.mcp.get_string("name")

# Check if space is already stopped
if not knot.space.is_running(space_name):
    knot.mcp.return_error("Space is already stopped")
else:
    success = knot.space.stop(space_name)

    if success:
        knot.mcp.return_object({"status": True})
    else:
        knot.mcp.return_error("Failed to stop space")
