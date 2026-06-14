import knot.space
import scriptling.mcp.tool as tool

# Optional show_all param: include spaces from all zones (default: current zone only)
show_all = tool.get_bool("show_all", False)

# List all spaces for the current user
spaces = knot.space.list(all_zones=show_all)

# Return as JSON
tool.return_object({"spaces": spaces})
