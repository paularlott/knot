#!/usr/bin/env python3
import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
file_path = tool.get_string("file_path")
content = knot.space.read_file(space_name, file_path)

tool.return_object({
    'file_path': file_path,
    'success': True,
    'content': content,
    'size': len(content)
})
