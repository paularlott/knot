import knot.space
import scriptling.mcp.tool as tool

name = tool.get_string("name")
template_name = tool.get_string("template_name")
description = tool.get_string("description", "")
shell = tool.get_string("shell", "bash")

space_id = knot.space.create(name, template_name, description=description, shell=shell)

tool.return_string(f"Space '{name}' created successfully with ID: {space_id}")
