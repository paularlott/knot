import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
file_path = tool.get_string("file_path")
search = tool.get_string("search")
replace = tool.get_string("replace")

bytes_written = knot.space.edit_file(space_name, file_path, search, replace)

tool.return_object({
    'file_path': file_path,
    'success': True,
    'bytes_written': bytes_written,
})
