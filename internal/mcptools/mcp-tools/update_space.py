import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
description = tool.get_string("description", "")
shell = tool.get_string("shell", "")

kwargs = {}
if description:
    kwargs["description"] = description
if shell:
    kwargs["shell"] = shell

success = knot.space.update(space_name, **kwargs)

if success:
  tool.return_string(f"Space '{space_name}' updated successfully")
else:
    tool.return_error("Failed to update space")
