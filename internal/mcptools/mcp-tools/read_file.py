#!/usr/bin/env python3
import knot.space
import knot.mcp

args = knot.mcp.get_args()
content = knot.space.read_file(args['space_name'], args['file_path'])

knot.mcp.return_object({
    'file_path': args['file_path'],
    'success': True,
    'content': content,
    'size': len(content)
})
