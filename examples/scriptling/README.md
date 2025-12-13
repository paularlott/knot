# Scriptling Examples

## Running Scripts

Scripts can be run from local files or by name from the server.

### Example Usage

```bash
# Run a local script file with arguments
knot run-script test_script.py arg1 arg2

# Run a script from the server by name
knot run-script scriptname arg1 arg2

# Specify server alias
knot run-script --alias production scriptname arg1 arg2
```

### Features

- **Local or server scripts**: Run from disk or fetch by name from server
- **On-demand library loading**: Import local .py files automatically
- **Server library fallback**: If library not found on disk, fetch from server
- **Pathlib support**: Access filesystem operations (agent/desktop only)
- **Dynamic libraries**: All server libraries downloaded when running server scripts

### Example Files

- `test_script.py` - Main script that imports mylib
- `mylib.py` - Library that will be loaded on-demand when imported
