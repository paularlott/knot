#!/usr/bin/env python3
import knot.template
import knot.mcp

args = knot.mcp.get_args()
knot.template.delete(args['template_name'])

knot.mcp.return_object({'status': True})
