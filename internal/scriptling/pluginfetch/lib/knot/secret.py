# knot.secret - Plugin transport variant
#
# Replaces the embedded knot.secret when the knot plugin serves it. Reads go
# through the plugin process, which asks the space's agent, so personal
# secrets work for scriptling using knot as a plugin inside a space and are
# refused anywhere else.

import scriptling.plugin as _plugin


def get(path, field=""):
    """Read one of your own secrets from 1Password.

    Args:
        path: vault/item, or a complete op://vault/item/field reference
        field: Field name, or section/field (default: password)

    Returns:
        The secret value as a string
    """
    return _plugin.call_function("plugin.knot", "user_secret", path, field)
