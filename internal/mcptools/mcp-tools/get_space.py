import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
space = knot.space.get(space_name)

tool.return_object(space)
