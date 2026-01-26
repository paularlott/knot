# Scriptling Skills Library

The `knot.skill` library provides functions to manage skills (knowledge base content) from within scriptling scripts. Skills are markdown documents with YAML or TOML frontmatter that follow the [Agent Skills Specification](https://agentskills.io/specification).

## Available Functions

| Function | Description |
|----------|-------------|
| `create(content, global=False, groups=[], zones=[])` | Create a new skill |
| `get(name_or_id)` | Get a skill by name or UUID |
| `update(name_or_id, content=None, groups=None, zones=None)` | Update an existing skill |
| `delete(name_or_id)` | Delete a skill |
| `list(owner=None)` | List all accessible skills |
| `search(query)` | Search skills by name and description |

## Key Concepts

- **Global Skills**: Available to all users (with group restrictions)
- **User Skills**: Personal skills owned by individual users
- **User Shadowing**: User skills with the same name override global skills
- **Zone Restrictions**: Skills can be limited to specific zones
- **Group Restrictions**: Global skills can be restricted to user groups
- **Permissions**: Two-tier permission system (global vs own)

## Functions

### create(content, global=False, groups=[], zones=[])

Create a new skill.

**Parameters:**

- `content` (string, required): Markdown content with frontmatter
- `global` (boolean, optional): If True, creates a global skill. Default: False (user skill)
- `groups` (list, optional): List of group IDs that can access this skill (global skills only)
- `zones` (list, optional): List of zones where this skill is available (empty = all zones)

**Returns:**

- `dict`: Dictionary with `skill_id` on success

**Frontmatter Requirements:**

```yaml
---
name: "my-skill"
description: "Brief description"
---
```

**Validation Rules:**

- `name`: 1-64 chars, lowercase letters/numbers/hyphens, must start with letter
- `description`: 1-1024 chars
- Content: Max 4MB total (including frontmatter)

**Permissions Required:**

- User skills: `MANAGE_OWN_SKILLS`
- Global skills: `MANAGE_GLOBAL_SKILLS`

**Example:**

```python
import knot.skill

# Create a user skill
skill = knot.skill.create("""---
name: "python-best-practices"
description: "Python coding best practices and patterns"
---

# Python Best Practices

## Code Style
- Follow PEP 8
- Use meaningful variable names
- Keep functions small and focused
""")
print(f"Created skill: {skill['skill_id']}")

# Create a global skill with group restrictions
skill = knot.skill.create("""---
name: "deployment-guide"
description: "Production deployment procedures"
---

# Deployment Guide
...""", global=True, groups=["dev-team-id", "ops-team-id"])

# Create a zone-specific skill
skill = knot.skill.create("""---
name: "aws-setup"
description: "AWS infrastructure setup guide"
---

# AWS Setup
...""", global=True, zones=["us-east-1", "us-west-2"])
```

### get(name_or_id)

Get a skill by name or UUID.

**Parameters:**

- `name_or_id` (string, required): Skill name or UUID

**Returns:**

- `dict`: Dictionary with skill details:
  - `skill_id` (string): UUID
  - `user_id` (string): User ID (empty for global skills)
  - `name` (string): Skill name
  - `description` (string): Skill description
  - `content` (string): Full markdown content with frontmatter
  - `groups` (list): List of group IDs (global skills only)
  - `zones` (list): List of zones (empty = all zones)
  - `is_managed` (bool): True if managed by parent server

**Name Resolution:**

- User skills shadow global skills (same name = user skill wins)
- Zone-specific skills override global skills

**Permissions Required:**

- User skills: `MANAGE_OWN_SKILLS`
- Global skills: `MANAGE_GLOBAL_SKILLS` or group membership

**Example:**

```python
import knot.skill

# Get by name
skill = knot.skill.get("python-best-practices")
print(skill["content"])

# Get by UUID
skill = knot.skill.get("550e8400-e29b-41d4-a716-446655440000")
print(f"{skill['name']}: {skill['description']}")
```

### update(name_or_id, content=None, groups=None, zones=None)

Update an existing skill.

**Parameters:**

- `name_or_id` (string, required): Skill name or UUID
- `content` (string, optional): New markdown content with frontmatter
- `groups` (list, optional): New list of group IDs (global skills only)
- `zones` (list, optional): New list of zones

**Returns:**

- `bool`: True on success

**Notes:**

- Use keyword arguments for optional parameters
- Frontmatter is re-extracted from content if provided
- Cannot update managed skills (on leaf nodes)
- User skills cannot have groups

**Permissions Required:**

- User skills: `MANAGE_OWN_SKILLS` (own skills only)
- Global skills: `MANAGE_GLOBAL_SKILLS`

**Example:**

```python
import knot.skill

# Update content only
knot.skill.update("python-best-practices", content="""---
name: "python-best-practices"
description: "Updated Python coding best practices"
---

# Python Best Practices (Updated)
...""")

# Update zones only
knot.skill.update("aws-setup", zones=["us-east-1", "eu-west-1"])

# Update multiple fields
knot.skill.update(
    "deployment-guide",
    content=new_content,
    groups=["dev-team-id"],
    zones=["production"]
)
```

### delete(name_or_id)

Delete a skill.

**Parameters:**

- `name_or_id` (string, required): Skill name or UUID

**Returns:**

- `bool`: True on success

**Notes:**

- Performs soft delete (skill marked as deleted)
- Cannot delete managed skills (on leaf nodes)
- Deletion propagates via gossip protocol

**Permissions Required:**

- User skills: `MANAGE_OWN_SKILLS` (own skills only)
- Global skills: `MANAGE_GLOBAL_SKILLS`

**Example:**

```python
import knot.skill

# Delete by name
knot.skill.delete("old-guide")

# Delete by UUID
knot.skill.delete("550e8400-e29b-41d4-a716-446655440000")
```

### list(owner=None)

List all accessible skills.

**Parameters:**

- `owner` (string, optional): Filter by owner ("user" or "global")

**Returns:**

- `list`: List of dictionaries with:
  - `name` (string): Skill name
  - `description` (string): Skill description

**Filtering:**

- Respects user permissions
- Respects group membership
- Respects zone restrictions
- Returns only accessible skills

**Permissions Required:**

- Returns skills based on user's permissions and group membership

**Example:**

```python
import knot.skill

# List all accessible skills
all_skills = knot.skill.list()
for skill in all_skills:
    print(f"{skill['name']}: {skill['description']}")

# List only user skills
my_skills = knot.skill.list(owner="user")

# List only global skills
global_skills = knot.skill.list(owner="global")
```

### search(query)

Search skills by name and description.

**Parameters:**

- `query` (string, required): Search query

**Returns:**

- `list`: List of dictionaries with:
  - `name` (string): Skill name
  - `description` (string): Skill description

**Search Behavior:**

- Fuzzy search on name and description
- Case-insensitive
- Respects user permissions
- Respects group membership
- Respects zone restrictions

**Permissions Required:**

- Returns skills based on user's permissions and group membership

**Example:**

```python
import knot.skill

# Search for Python-related skills
results = knot.skill.search("python")
for skill in results:
    print(f"Found: {skill['name']}")

# Search for deployment guides
results = knot.skill.search("deployment")
```

## Permission Constants

The `knot.permission` library provides constants for skill permissions:

```python
knot.permission.MANAGE_OWN_SKILLS      # Create, update, delete own skills
knot.permission.MANAGE_GLOBAL_SKILLS   # Create, update, delete any skill
```

**Example:**

```python
import knot.permission

# Check if user has permission
if user.has_permission(knot.permission.MANAGE_GLOBAL_SKILLS):
    knot.skill.create(content, global=True)
```

## Complete Example

```python
import knot.skill

# Create a comprehensive skill management script

# Create a new skill
skill_content = """---
name: "kubernetes-troubleshooting"
description: "Common Kubernetes troubleshooting procedures"
---

# Kubernetes Troubleshooting

## Pod Issues
1. Check pod status: `kubectl get pods`
2. View logs: `kubectl logs <pod-name>`
3. Describe pod: `kubectl describe pod <pod-name>`

## Network Issues
1. Check services: `kubectl get svc`
2. Test connectivity: `kubectl exec -it <pod> -- curl <service>`
"""

# Create the skill
result = knot.skill.create(
    skill_content,
    global=True,
    groups=["devops-team"],
    zones=["production", "staging"]
)
print(f"Created skill: {result['skill_id']}")

# List all skills
print("\nAll accessible skills:")
for skill in knot.skill.list():
    print(f"  - {skill['name']}: {skill['description']}")

# Search for troubleshooting guides
print("\nTroubleshooting guides:")
for skill in knot.skill.search("troubleshooting"):
    print(f"  - {skill['name']}")

# Get and display a specific skill
skill = knot.skill.get("kubernetes-troubleshooting")
print(f"\n{skill['name']}:")
print(skill['content'])

# Update the skill
updated_content = skill['content'] + "\n\n## Storage Issues\n..."
knot.skill.update("kubernetes-troubleshooting", content=updated_content)

# Clean up old skills
old_skills = ["deprecated-guide", "old-tutorial"]
for name in old_skills:
    try:
        knot.skill.delete(name)
        print(f"Deleted: {name}")
    except Exception as e:
        print(f"Could not delete {name}: {e}")
```

## Related Libraries

- **knot.ai** - For AI completion and response functions
- **knot.mcp** - For direct MCP tool access
- **knot.permission** - For permission constants
