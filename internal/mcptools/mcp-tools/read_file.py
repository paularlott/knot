#!/usr/bin/env python3
import knot.space
import knot.mcp

space_name = knot.mcp.get_string("space_name")
file_path = knot.mcp.get_string("file_path")
content = knot.space.read_file(space_name, file_path)

knot.mcp.return_object({
    'file_path': file_path,
    'success': True,
    'content': content,
    'size': len(content)
})
