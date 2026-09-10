import knot.template
import scriptling.mcp.tool as tool

templates = knot.template.list(resolve_options=True)

# The caller (an LLM) can only use option values, not handler ids: once the
# options are resolved the handler is noise. A field whose handler could not
# be reached keeps its handler and has no options — dynamic, currently
# unavailable — which the API's validation error will also name.
for template in templates:
    for field in template.get("custom_fields", []):
        if "options" in field and "handler" in field:
            del field["handler"]

tool.return_object({"templates": templates})
