#!/usr/bin/env python3
import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
file_path = tool.get_string("file_path")
content = tool.get_string("content")
knot.space.write_file(space_name, file_path, content)

tool.return_object({
    'file_path': file_path,
    'success': True,
    'message': f"Successfully wrote to {file_path}",
    'bytes_written': len(content)
})
