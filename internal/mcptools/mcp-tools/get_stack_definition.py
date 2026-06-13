import knot.stack
import scriptling.mcp.tool as tool

name = tool.get_string("name")
tool.return_object(knot.stack.get_def(name))

