import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
pattern = tool.get_string("pattern")
path = tool.get_string("path")
literal = tool.get_bool("literal", False)
recursive = tool.get_bool("recursive", False)
ignore_case = tool.get_bool("ignore_case", False)
glob = tool.get_string("glob", "")

matches = knot.space.grep(
    space_name, pattern, path,
    literal=literal,
    recursive=recursive,
    ignore_case=ignore_case,
    glob=glob,
)

tool.return_object({
    "success": True,
    "pattern": pattern,
    "path": path,
    "matches": matches,
    "count": len(matches),
})
