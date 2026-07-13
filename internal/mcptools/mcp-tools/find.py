import knot.space
import scriptling.mcp.tool as tool

space_name = tool.get_string("name")
path = tool.get_string("path", ".")
name_glob = tool.get_string("name_glob", "")
entry_type = tool.get_string("type", "any")
recursive = tool.get_bool("recursive", True)
include_hidden = tool.get_bool("include_hidden", False)
max_depth = tool.get_int("max_depth", 0)

paths = knot.space.find(
    space_name,
    path=path,
    recursive=recursive,
    type=entry_type,
    name_glob=name_glob,
    include_hidden=include_hidden,
    max_depth=max_depth,
)

tool.return_object({
    "success": True,
    "path": path,
    "paths": paths,
    "count": len(paths),
})
