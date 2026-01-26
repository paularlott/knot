import knot.user
import knot.mcp

users = knot.user.list()
knot.mcp.return_object({"users": users})
