# knot.skill - Skill management library for Knot server
#
# This library provides functions for managing skills in Knot.
# Requires knot.apiclient to be configured first.
#
# Usage:
#   import knot.apiclient
#   import knot.skill
#
#   knot.apiclient.configure("https://knot.example.com", "your-token")
#   skills = knot.skill.list()

import knot.apiclient as api

def list(owner=None):
    """List all skills the user has access to.

    Args:
        owner: Filter by owner user ID (optional)

    Returns:
        A list of skill dicts, each containing:
        - id: Skill ID
        - name: Skill name
        - description: Skill description
        - user_id: Owner user ID
        - is_managed: Boolean indicating if skill is managed

    Raises:
        Exception if not configured or on API error
    """
    url = "api/skill?all_zones=true"
    if owner:
        url = f"api/skill?user_id={owner}&all_zones=true"

    response = api.get(f"/{url}")

    result = []
    for skill in response.get("skills", []):
        result.append({
            "id": skill.get("id"),
            "name": skill.get("name"),
            "description": skill.get("description", ""),
            "user_id": skill.get("user_id"),
            "is_managed": skill.get("is_managed", False)
        })

    return result


def get(name_or_id):
    """Get skill by name or UUID.

    Args:
        name_or_id: Skill name or UUID

    Returns:
        A dict containing skill details:
        - id: Skill ID
        - name: Skill name
        - description: Skill description
        - content: Skill content
        - user_id: Owner user ID
        - is_managed: Boolean indicating if skill is managed
        - groups: List of group names
        - zones: List of zone names

    Raises:
        Exception if not configured or on API error
    """
    # Try as UUID first
    try:
        response = api.get(f"/api/skill/{name_or_id}")
    except Exception:
        response = api.get(f"/api/skill/name/{name_or_id}")

    return {
        "id": response.get("id"),
        "name": response.get("name"),
        "description": response.get("description", ""),
        "content": response.get("content", ""),
        "user_id": response.get("user_id"),
        "is_managed": response.get("is_managed", False),
        "groups": response.get("groups", []),
        "zones": response.get("zones", [])
    }


def create(content, is_global=False, groups=None, zones=None):
    """Create a new skill.

    Args:
        content: Skill content (markdown/text)
        global: If True, create as global skill (default: False)
        groups: List of group names (optional)
        zones: List of zone names (optional)

    Returns:
        The new skill ID

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "content": content,
        "groups": groups or [],
        "zones": zones or [],
        "active": True
    }

    if is_global:
        body["user_id"] = ""

    response = api.post("/api/skill", body)
    return response.get("id")


def update(name_or_id, content=None, groups=None, zones=None):
    """Update skill properties.

    Args:
        name_or_id: Skill name or UUID
        content: New skill content (optional)
        groups: List of group names (optional)
        zones: List of zone names (optional)

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    # Get current skill
    try:
        current = api.get(f"/api/skill/{name_or_id}")
    except Exception:
        current = api.get(f"/api/skill/name/{name_or_id}")

    body = {
        "content": content if content is not None else current.get("content", ""),
        "groups": groups if groups is not None else current.get("groups", []),
        "zones": zones if zones is not None else current.get("zones", [])
    }

    api.put(f"/api/skill/{current.get('id')}", body)
    return True


def delete(name_or_id):
    """Delete a skill.

    Args:
        name_or_id: Skill name or UUID

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    # Get skill to find UUID
    try:
        skill = api.get(f"/api/skill/{name_or_id}")
    except Exception:
        skill = api.get(f"/api/skill/name/{name_or_id}")

    api.delete(f"/api/skill/{skill.get('id')}")
    return True


def search(query):
    """Fuzzy search skills by name/description.

    Args:
        query: Search query string

    Returns:
        A list of matching skill dicts

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/skill/search", {"q": query, "all_zones": "true"})

    result = []
    for skill in response.get("skills", []):
        result.append({
            "id": skill.get("id"),
            "name": skill.get("name"),
            "description": skill.get("description", ""),
            "user_id": skill.get("user_id")
        })

    return result
