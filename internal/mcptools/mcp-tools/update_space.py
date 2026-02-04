import knot.space
import knot.mcp

space_name = knot.mcp.get_string("name")
new_name = knot.mcp.get_string("new_name", "")
description = knot.mcp.get_string("description", "")
shell = knot.mcp.get_string("shell", "")

kwargs = {}
if new_name:
    kwargs["name"] = new_name
if description:
    kwargs["description"] = description
if shell:
    kwargs["shell"] = shell

success = knot.space.update(space_name, **kwargs)

if success:
    knot.mcp.return_object({"status": True})
else:
    knot.mcp.return_error("Failed to update space")
