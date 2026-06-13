import knot.stack
import scriptling.mcp.tool as tool

stack_name = tool.get_string("stack_name")
knot.stack.restart(stack_name)
tool.return_string(f"Stack '{stack_name}' restarted successfully")

