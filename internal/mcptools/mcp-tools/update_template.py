#!/usr/bin/env python3
import knot.template
import knot.mcp

args = knot.mcp.get_args()

# Build kwargs for template.update()
kwargs = {}
for key in ['name', 'job', 'description', 'platform', 'volumes', 'active', 'compute_units', 'storage_units',
            'with_terminal', 'with_vscode_tunnel', 'with_code_server', 'with_ssh', 'with_run_command',
            'schedule_enabled', 'icon_url', 'groups', 'zones', 'schedule', 'custom_fields']:
    if key in args:
        kwargs[key] = args[key]

knot.template.update(args['template_name'], **kwargs)

knot.mcp.return_object({'status': True})
