import knot.space
import knot.mcp

space_name = knot.mcp.get("space_name")
space = knot.space.get(space_name)

knot.mcp.return_object(space)
