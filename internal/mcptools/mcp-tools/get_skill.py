import knot.apiclient as api
import scriptling.mcp.tool as tool

name = tool.get_string("name", "")
query = tool.get_string("query", "")

if name:
    try:
        skill = api.get(f"/api/skill/{name}")
        tool.return_object({"skill": skill["content"], "score": 1.0})
    except Exception:
        if query:
            tool.return_object({"results": api.get(f"/api/skill/search?q={query}")})
        else:
            tool.return_error(f"Skill not found: {name}")
elif query:
    tool.return_object({"results": api.get(f"/api/skill/search?q={query}")})
else:
    response = api.get("/api/skill")
    skills = [{"name": s["name"], "description": s["description"]} for s in response.get("skills", []) if s.get("active")]
    tool.return_object({"action": "list", "count": len(skills), "skills": skills})
