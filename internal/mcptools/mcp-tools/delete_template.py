import knot.template
import scriptling.mcp.tool as tool

template_id = tool.get_string("template_id")
success = knot.template.delete(template_id)

if success:
    tool.return_string(f"Template '{template_id}' deleted successfully")
else:
    tool.return_error(f"Failed to delete template '{template_id}'")
