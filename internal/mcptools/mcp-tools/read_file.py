#!/usr/bin/env python3
import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
file_path = tool.get_string("file_path")
offset = tool.get_int("offset", 0)
limit = tool.get_int("limit", 0)

content = knot.space.read_file(space_name, file_path, offset=offset, limit=limit)

result = {
    'file_path': file_path,
    'success': True,
    'content': content,
    'size': len(content)
}
if offset > 0 or limit > 0:
    result['offset'] = offset
    result['limit'] = limit

tool.return_object(result)
