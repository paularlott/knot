import knot.server
import knot.space
import knot.template
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
try:
    space = knot.space.get(space_name)
except Exception:
    tool.return_error(f"Space '{space_name}' not found")

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

        username = space.get("username", "")
        template_name = space.get("template_name", "") or space.get("template_id", "")

        if username and template_name:
            template = knot.template.get(template_name)
            for port in template.get("ports", []):
                protocol = port.get("protocol", "")
                port_number = int(port.get("port", 0))
                port_name = port.get("name", "")
                if protocol in ("http", "https") and port_number:
                    urls.append({
                        "name": port_name,
                        "port": port_number,
                        "protocol": protocol,
                        "url": f"https://{username.lower()}--{space_name.lower()}--{port_number}{domain}",
                    })
except Exception:
    pass

if urls:
    space["urls"] = urls

tool.return_object(space)
