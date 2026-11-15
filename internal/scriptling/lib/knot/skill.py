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
import urllib.parse


def _enc(s):
    """URL-encode a path segment for safe interpolation into a URL."""
    return urllib.parse.quote(str(s), safe='')

def _parse_skill(skill):
    """Parse a skill response (the API's id key is "skill_id") into a stable dict."""
    return {
        "id": skill.get("skill_id", skill.get("id")),
        "name": skill.get("name"),
        "description": skill.get("description", ""),
        "user_id": skill.get("user_id", ""),
        "is_managed": skill.get("is_managed", False),
        "active": skill.get("active", False),
        "groups": skill.get("groups") or [],
        "zones": skill.get("zones") or [],
    }


def _fetch(name_or_id):
    """Fetch a skill's raw details. GET /api/skill/{x} accepts a UUID or a
    name; names resolve the current user's own skill before a global one."""
    return api.get(f"/api/skill/{_enc(name_or_id)}")


def _frontmatter(content):
    """Return (name, description) from a skill's --- or +++ frontmatter."""
    lines = str(content or "").split("\n")
    if not lines or lines[0].strip() not in ("---", "+++"):
        return "", ""
    delimiter = lines[0].strip()
    name = ""
    description = ""
    for line in lines[1:]:
        line = line.strip()
        if line == delimiter:
            break
        for sep in (":", "="):
            idx = line.find(sep)
            if idx > 0:
                key = line[:idx].strip().lower()
                value = line[idx + 1:].strip().strip("\"'")
                if key == "name":
                    name = value
                elif key == "description":
                    description = value
                break
    return name, description


def list(owner=None):
    """List all skills the user has access to.

    Args:
        owner: Filter by owner user ID (optional). "current" lists only the
            current user's own skills.

    Returns:
        A list of skill dicts, each containing:
        - id: Skill ID
        - name: Skill name
        - description: Skill description
        - user_id: Owner user ID (empty for a global skill)
        - is_managed: Boolean indicating if skill is managed
        - active: Boolean indicating if skill is active
        - groups: List of group IDs (global skills only)
        - zones: List of zone names

    Raises:
        Exception if not configured or on API error
    """
    params = {"all_zones": "true"}
    if owner:
        if owner == "current":
            owner = api.get("/api/users/whoami").get("user_id", "")
        params["user_id"] = owner

    response = api.get("/api/skill", params)
    return [_parse_skill(skill) for skill in (response.get("skills") or [])]


def get(name_or_id):
    """Get skill by name or UUID.

    Args:
        name_or_id: Skill name or UUID. A name resolves to the current
            user's own skill first, then a global skill.

    Returns:
        A dict containing skill details:
        - id: Skill ID
        - name: Skill name
        - description: Skill description
        - content: Skill content
        - user_id: Owner user ID (empty for a global skill)
        - is_managed: Boolean indicating if skill is managed
        - active: Boolean indicating if skill is active
        - groups: List of group IDs
        - zones: List of zone names

    Raises:
        Exception if not configured or on API error
    """
    response = _fetch(name_or_id)
    result = _parse_skill(response)
    result["content"] = response.get("content", "")
    return result


def create(content, is_global=False, groups=None, zones=None, owner=None, active=True):
    """Create a new skill.

    The name and description come from the content's frontmatter.

    Args:
        content: Skill content (markdown with --- name/description frontmatter)
        is_global: If True, create a global skill (needs the Manage Global
            Skills permission). Default False: a user skill owned by `owner`.
        groups: List of group IDs (optional; global skills only)
        zones: List of zone names (optional)
        owner: Owner user ID for a user skill (default: the current user;
            another user's needs Manage Global Skills). Ignored when is_global.
        active: Whether the skill is active (default True)

    Returns:
        The new skill ID

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "user_id": "" if is_global else (owner or "current"),
        "content": content,
        "groups": groups or [],
        "zones": zones or [],
        "active": active,
    }

    response = api.post("/api/skill", body)
    return response.get("skill_id")


def update(name_or_id, content=None, groups=None, zones=None, active=None):
    """Update skill properties.

    Only the given arguments change; the rest keep their current values.

    Args:
        name_or_id: Skill name or UUID
        content: New skill content (optional; its frontmatter sets name and description)
        groups: List of group IDs (optional; global skills only)
        zones: List of zone names (optional)
        active: Whether the skill is active (optional)

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    current = _fetch(name_or_id)

    body = {
        "content": content if content is not None else current.get("content", ""),
        "groups": groups if groups is not None else (current.get("groups") or []),
        "zones": zones if zones is not None else (current.get("zones") or []),
        "active": active if active is not None else current.get("active", False),
    }

    api.put(f"/api/skill/{_enc(current.get('skill_id'))}", body)
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
    skill = _fetch(name_or_id)
    api.delete(f"/api/skill/{_enc(skill.get('skill_id'))}")
    return True


def search(query):
    """Fuzzy search skills by name/description.

    Args:
        query: Search query string

    Returns:
        Up to 3 matching skill dicts, best first, each containing:
        - id, name, description, user_id: as returned by listing (id/user_id are None/""
          if the skill can no longer be fetched)
        - content: Skill content
        - score: Match score from 0 to 1

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/skill/search", {"q": query, "all_zones": "true"})

    result = []
    for match in (response or []):
        content = match.get("skill", "")
        name, description = _frontmatter(content)
        entry = {
            "id": None,
            "name": name,
            "description": description,
            "user_id": "",
            "content": content,
            "score": match.get("score", 0),
        }
        if name:
            try:
                details = _fetch(name)
                # A same-named skill can shadow this one; only claim its
                # id if it is the skill the search matched.
                if details.get("content", "") == content:
                    entry["id"] = details.get("skill_id")
                    entry["user_id"] = details.get("user_id", "")
            except Exception:
                pass
        result.append(entry)

    return result
