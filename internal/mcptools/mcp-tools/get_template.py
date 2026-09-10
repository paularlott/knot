import knot.template
import scriptling.mcp.tool as tool

template_name = tool.get_string("template")
try:
    template = knot.template.get(template_name, resolve_options=True)
except Exception as e:
    tool.return_error(str(e))

# Same discovery shape as list_templates, with handler-backed options
# resolved as the requesting user. Once the options are known the handler id
# is noise — the caller can only use option values. A field whose handler
# could not be reached keeps its handler and has no options.
fields = []
for field in template.get("custom_fields", []):
    if "options" in field and "handler" in field:
        del field["handler"]
    fields.append(field)

tool.return_object({
    "id": template.get("id"),
    "name": template.get("name"),
    "description": template.get("description", ""),
    "platform": template.get("platform", ""),
    "active": template.get("active", False),
    "custom_fields": fields,
})
