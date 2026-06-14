import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
new_name = tool.get_string("new_name", "")
description = tool.get_string("description", "")
shell = tool.get_string("shell", "")
custom_field_args = tool.get_string_list("custom_fields", [])

custom_fields = []
for custom_field in custom_field_args:
    if isinstance(custom_field, dict):
        if "name" in custom_field:
            field_name = custom_field.get("name")
            field_value = custom_field.get("value")
            if not field_name or not isinstance(field_name, str) or not field_name.strip():
                tool.return_error("custom_fields entries must include a field name")
            custom_fields.append({"name": field_name.strip(), "value": str(field_value) if field_value is not None else ""})
        else:
            for k, v in custom_field.items():
                if not isinstance(k, str) or not k.strip():
                    tool.return_error("custom_fields keys must be non-empty strings")
                custom_fields.append({"name": k.strip(), "value": str(v) if v is not None else ""})
    elif isinstance(custom_field, str):
        if "=" not in custom_field:
            tool.return_error("custom_fields entries must use name=value format (e.g. \"ExcludeDebug=123\"), {\"name\": ..., \"value\": ...}, or {\"ExcludeDebug\": \"123\"}")
        field_name, field_value = custom_field.split("=", 1)
        if not field_name.strip():
            tool.return_error("custom_fields entries must include a field name")
        custom_fields.append({"name": field_name.strip(), "value": field_value})
    else:
        tool.return_error("custom_fields entries must be strings or dicts")

kwargs = {}
if new_name:
    kwargs["new_name"] = new_name
if description:
    kwargs["description"] = description
if shell:
    kwargs["shell"] = shell
if custom_fields:
    kwargs["custom_fields"] = custom_fields

success = knot.space.update(space_name, **kwargs)

if success:
  tool.return_string(f"Space '{space_name}' updated successfully")
else:
    tool.return_error("Failed to update space")
