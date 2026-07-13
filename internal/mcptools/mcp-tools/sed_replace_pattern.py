import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
pattern = tool.get_string("pattern")
new = tool.get_string("new")
path = tool.get_string("path")
recursive = tool.get_bool("recursive", False)
ignore_case = tool.get_bool("ignore_case", False)
glob = tool.get_string("glob", "")

count = knot.space.sed_replace_pattern(
    space_name, pattern, new, path,
    recursive=recursive,
    ignore_case=ignore_case,
    glob=glob,
)

tool.return_object({
    "success": True,
    "path": path,
    "files_modified": count,
})
