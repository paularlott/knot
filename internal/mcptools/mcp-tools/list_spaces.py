import knot.space
import knot.mcp

# List all spaces for the current user
spaces = knot.space.list()

# Return as JSON
knot.mcp.return_object({"spaces": spaces})
