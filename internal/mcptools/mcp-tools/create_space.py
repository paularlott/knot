import knot.space
import knot.mcp

name = knot.mcp.get("name")
template_name = knot.mcp.get("template_name")
description = knot.mcp.get("description", "")
shell = knot.mcp.get("shell", "bash")

space_id = knot.space.create(name, template_name, description=description, shell=shell)

knot.mcp.return_object({"status": True, "space_id": space_id})
