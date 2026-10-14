# knot.secret - Personal secrets library for Knot server
#
# Reads the space owner's own secrets with the 1Password service account
# token stored on their profile (Knot Pro). Secrets are only ever read inside
# a knot space, through the space's agent: scripts run by the agent and
# scriptling using knot as a plugin in a space get a working get(). Anywhere
# else — server-side scripts, or outside a space — this module refuses.
#
# The token is set in the knot web interface.
#
# Usage (inside a space):
#   import knot.secret
#
#   username = knot.secret.get("op://vault/item/username")
#   api_key = knot.secret.get("op://vault/item/section/api key")


def get(path, field=""):
    """Read one of your own secrets from 1Password.

    Args:
        path: The secret's 1Password reference, op://vault/item/field, or
            op://vault/item/section/field for a field in a section

    Returns:
        The secret value as a string

    Raises:
        Exception always here: personal secrets can only be read inside a
        knot space, where the agent provides this function
    """
    raise Exception("personal secrets can only be read inside a knot space")
