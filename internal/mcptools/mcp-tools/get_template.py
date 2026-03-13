import knot.template
import scriptling.mcp.tool as tool

template_id = tool.get_string("template_id")
template = knot.template.get(template_id)

tool.return_object(template)
