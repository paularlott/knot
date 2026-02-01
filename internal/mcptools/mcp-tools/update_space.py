import knot.space
import knot.mcp

space_name = knot.mcp.get("name")
new_name = knot.mcp.get("new_name", None)
description = knot.mcp.get("description", None)
shell = knot.mcp.get("shell", None)

kwargs = {}
if new_name is not None:
    kwargs["name"] = new_name
if description is not None:
    kwargs["description"] = description
if shell is not None:
    kwargs["shell"] = shell

success = knot.space.update(space_name, **kwargs)

if success:
    knot.mcp.return_object({"status": True})
else:
    knot.mcp.return_error("Failed to update space")
