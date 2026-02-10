import knot.space
import scriptling.mcp.tool as tool

# List all spaces for the current user
spaces = knot.space.list()

# Return as JSON
tool.return_object(spaces)
