import knot.space
import scriptling.mcp.tool as tool

name = tool.get_string("name")
template_name = tool.get_string("template_name")
description = tool.get_string("description", "")
shell = tool.get_string("shell", "bash")
custom_field_args = tool.get_string_list("custom_fields", [])
start_on_create = tool.get_bool("start_on_create", False)

custom_fields = []
for custom_field in custom_field_args:
    if "=" not in custom_field:
        tool.return_error("custom_fields entries must use name=value format")

    field_name, field_value = custom_field.split("=", 1)
    field_name = field_name.strip()
    if not field_name:
        tool.return_error("custom_fields entries must include a field name")

    custom_fields.append({"name": field_name, "value": field_value})

space_id = knot.space.create(
    name,
    template_name,
    description=description,
    shell=shell,
    custom_fields=custom_fields,
    start_on_create=start_on_create,
)

message = f"Space '{name}' created successfully with ID: {space_id}"
if start_on_create:
    message += " and started"

tool.return_string(message)
