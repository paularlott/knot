import knot.stack
import scriptling.mcp.tool as tool

definition_name = tool.get_string("definition_name")
prefix = tool.get_string("prefix")
stack_name = tool.get_string("stack_name", "")

result = knot.stack.create(
    definition_name,
    prefix,
    stack_name=stack_name if stack_name else None,
)

tool.return_object({
    "success": True,
    "stack": stack_name if stack_name else prefix,
    "spaces": result.get("spaces", {}),
})

