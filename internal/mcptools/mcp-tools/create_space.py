import knot.server
import knot.space
import knot.template
import scriptling.mcp.tool as tool

name = tool.get_string("name")
template_name = tool.get_string("template_name")
description = tool.get_string("description", "")
shell = tool.get_string("shell", "bash")
custom_field_args = tool.get_string_list("custom_fields", [])
start_on_create = tool.get_bool("start_on_create", False)

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

urls = []
try:
    server_info = knot.server.info()
    wildcard_domain = server_info.get("wildcard_domain", "")
    if wildcard_domain:
        domain = wildcard_domain
        if domain.startswith("*"):
            domain = domain[1:]
        elif not domain.startswith("."):
            domain = "." + domain

        template = knot.template.get(template_name)
        space = knot.space.get(name)
        username = space.get("username", "")

        if username:
            for port in template.get("ports", []):
                protocol = port.get("protocol", "")
                port_number = int(port.get("port", 0))
                port_name = port.get("name", "")
                if protocol in ("http", "https") and port_number:
                    urls.append({
                        "name": port_name,
                        "port": port_number,
                        "protocol": protocol,
                        "url": f"https://{username.lower()}--{name.lower()}--{port_number}{domain}",
                    })
except Exception:
    pass

result = {"message": message}
if urls:
    result["urls"] = urls

tool.return_object(result)
