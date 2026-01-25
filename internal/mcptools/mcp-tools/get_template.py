#!/usr/bin/env python3
import knot.template
import knot.mcp

args = knot.mcp.get_args()
template = knot.template.get(args['template_name'])

knot.mcp.return_object(template)
