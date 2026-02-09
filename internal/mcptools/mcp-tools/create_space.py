import knot.space
import knot.mcp

name = knot.mcp.get_string("name")
template_name = knot.mcp.get_string("template_name")
description = knot.mcp.get_string("description", "")
shell = knot.mcp.get_string("shell", "bash")

space_id = knot.space.create(name, template_name, description=description, shell=shell)

knot.mcp.return_string(f"Space '{name}' created successfully with ID: {space_id}")
