# knot.permission - Permission constants library for Knot server
#
# This library provides permission constants and functions for checking permissions.
# Requires knot.apiclient to be configured first.
#
# Usage:
#   import knot.apiclient
#   import knot.permission
#
#   knot.apiclient.configure("https://knot.example.com", "your-token")
#   perms = knot.permission.list()
#
#   # Use constants with the role/user APIs that take permission ids
#   perms = knot.permission.list()
#   print(knot.permission.MANAGE_SPACES in [p["id"] for p in perms])

import knot.apiclient as api

# User Management
MANAGE_USERS = 0
MANAGE_GROUPS = 4
MANAGE_ROLES = 5

# Resource Management
MANAGE_SPACES = 2
MANAGE_TEMPLATES = 1
MANAGE_VOLUMES = 3
MANAGE_VARIABLES = 6

# Space Operations
USE_SPACES = 7
TRANSFER_SPACES = 10
SHARE_SPACES = 11
USE_TUNNELS = 8
EDIT_SPACE_JOBS = 43

# System & Audit
VIEW_AUDIT_LOGS = 9
CLUSTER_INFO = 12

# Space Features
USE_VNC = 13
USE_WEB_TERMINAL = 14
USE_SSH = 15
USE_CODE_SERVER = 16
USE_VSCODE_TUNNEL = 17
USE_LOGS = 18
RUN_COMMANDS = 19
COPY_FILES = 20

# AI Tools
USE_MCP_SERVER = 21
USE_WEB_ASSISTANT = 22

# Scripting
MANAGE_SCRIPTS = 23
EXECUTE_SCRIPTS = 24
MANAGE_OWN_SCRIPTS = 25
EXECUTE_OWN_SCRIPTS = 26

# Skills
MANAGE_GLOBAL_SKILLS = 27
MANAGE_OWN_SKILLS = 28

# Space Forms & Templates
SET_SPACE_DEPENDENCIES = 29
USE_SPACE_STARTUP_SCRIPT = 30

# Audit
DOWNLOAD_AUDIT_LOGS = 31

# Stack Definitions
MANAGE_STACK_DEFINITIONS = 32
MANAGE_OWN_STACK_DEFINITIONS = 33
USE_STACK_DEFINITIONS = 34

# Methods & Pools
USE_METHODS = 35
USE_POOLS = 36

# Events
MANAGE_EVENTS = 37
MANAGE_GLOBAL_EVENTS = 38

# Slash Commands
MANAGE_GLOBAL_SLASH_COMMANDS = 39
MANAGE_OWN_SLASH_COMMANDS = 40

# MCP Servers
MANAGE_MCP_SERVERS = 41

# Plugins
VIEW_PLUGINS = 44

# Aliases for convenience
SPACE_MANAGE = MANAGE_SPACES
SPACE_USE = USE_SPACES
SCRIPT_MANAGE = MANAGE_SCRIPTS
SCRIPT_EXECUTE = EXECUTE_SCRIPTS


def list():
    """List all permissions with their IDs, names, and groups.

    Returns:
        A list of permission dicts, each containing:
        - id: Permission ID (integer)
        - name: Permission name (string)
        - group: Permission group name (string)

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/permissions")

    result = []
    for perm in response.get("permissions", []):
        result.append({
            "id": perm.get("id"),
            "name": perm.get("name"),
            "group": perm.get("group")
        })

    return result
