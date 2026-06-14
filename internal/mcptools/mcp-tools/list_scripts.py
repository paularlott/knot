import knot.script
import scriptling.mcp.tool as tool

scripts = knot.script.list(script_type="script")

tool.return_object({
    "scripts": [
        {"name": s["name"], "description": s["description"]}
        for s in scripts
    ]
})
