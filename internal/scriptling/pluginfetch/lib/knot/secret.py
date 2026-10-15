# knot.secret - Plugin transport variant
#
# Replaces the embedded knot.secret when the knot plugin serves it. Reads go
# through the plugin process, which asks the space's agent, so personal
# secrets work for scriptling using knot as a plugin inside a space and are
# refused anywhere else.

import scriptling.plugin as _plugin


def get(path):
    """Read one of your own secrets from 1Password.

    Args:
        path: The secret's 1Password reference, op://vault/item/field, or
            op://vault/item/section/field for a field in a section

    Returns:
        The secret value as a string
    """
    return _plugin.call_function("plugin.knot", "user_secret", path)
