# Scriptling Volume Library

The `knot.volume` library provides volume management functions for scriptling scripts. This library is available in Local and Remote environments.

## Overview

Volumes are persistent storage resources that can be attached to spaces. They provide durable storage that persists across space restarts and recreations.

## Available Functions

| Function | Description |
|----------|-------------|
| `list()` | List all volumes |
| `get(volume_id)` | Get volume by ID or name |
| `create(name, definition, platform='')` | Create a new volume |
| `update(volume_id, ...)` | Update volume properties |
| `delete(volume_id)` | Delete a volume |
| `start(volume_id)` | Start a volume |
| `stop(volume_id)` | Stop a volume |
| `is_running(volume_id)` | Check if volume is running |

## Usage

```python
import knot.volume

# List all volumes
volumes = knot.volume.list()
for v in volumes:
    print(f"{v['name']}: {'running' if v['active'] else 'stopped'}")

# Create a new volume
volume_id = knot.volume.create(
    name="data-volume",
    definition="size: 10GB",
    platform="linux/amd64"
)

# Start the volume
knot.volume.start(volume_id)
```

## Functions

### list()

List all volumes.

**Parameters:** None

**Returns:**

- `list`: List of volume objects, each containing:
  - `id` (string): Volume ID
  - `name` (string): Volume name
  - `active` (bool): Whether the volume is running
  - `zone` (string): Volume zone
  - `platform` (string): Volume platform

**Example:**

```python
import knot.volume

# List all volumes
volumes = knot.volume.list()

print(f"Total volumes: {len(volumes)}")
for vol in volumes:
    status = "running" if vol['active'] else "stopped"
    print(f"- {vol['name']}: {status} ({vol['zone']})")
```

---

### get(volume_id)

Get a volume by ID or name.

**Parameters:**

- `volume_id` (string): Volume ID or name

**Returns:**

- `dict`: Volume object containing:
  - `id` (string): Volume ID
  - `name` (string): Volume name
  - `definition` (string): Volume definition
  - `active` (bool): Whether running
  - `zone` (string): Volume zone
  - `platform` (string): Volume platform

**Example:**

```python
import knot.volume

# Get volume by ID
vol = knot.volume.get("vol_abc123...")
print(f"Name: {vol['name']}")
print(f"Definition: {vol['definition']}")

# Get volume by name
vol = knot.volume.get("data-volume")
print(f"Volume ID: {vol['id']}")
print(f"Running: {vol['active']}")
```

---

### create(name, definition, platform='')

Create a new volume.

**Parameters:**

- `name` (string): Volume name
- `definition` (string): Volume definition (size, type, etc.)

**Optional Keyword Arguments:**

- `platform` (string): Platform (default: "")

**Returns:**

- `string`: The ID of the newly created volume

**Example:**

```python
import knot.volume

# Create a basic volume
volume_id = knot.volume.create(
    name="data-volume",
    definition="size: 10GB"
)
print(f"Created volume: {volume_id}")

# Create a volume with platform
volume_id = knot.volume.create(
    name="app-storage",
    definition="size: 50GB, type: ssd",
    platform="linux/amd64"
)
print(f"Created volume: {volume_id}")

# Create a database volume
volume_id = knot.volume.create(
    name="postgres-data",
    definition="size: 100GB, type: nvme, iops: 5000",
    platform="linux/amd64"
)
print(f"Created database volume: {volume_id}")
```

---

### update(volume_id, ...)

Update a volume's properties.

**Parameters:**

- `volume_id` (string): Volume ID or name

**Optional Keyword Arguments:**

- `name` (string): New volume name
- `definition` (string): New volume definition
- `platform` (string): New platform

**Returns:**

- `bool`: True if successfully updated, raises error on failure

**Example:**

```python
import knot.volume

# Update volume name
knot.volume.update("data-volume", name="production-data")

# Update volume definition (increase size)
knot.volume.update(
    "data-volume",
    definition="size: 20GB, type: ssd"
)

# Update multiple properties
knot.volume.update(
    "app-storage",
    name="production-storage",
    definition="size: 100GB, type: nvme",
    platform="linux/amd64"
)
```

---

### delete(volume_id)

Delete a volume.

**Parameters:**

- `volume_id` (string): Volume ID or name

**Returns:**

- `bool`: True if successfully deleted, raises error on failure

**Example:**

```python
import knot.volume

# Delete a volume
if knot.volume.delete("old-volume"):
    print("Volume deleted successfully")
```

---

### start(volume_id)

Start a stopped volume.

**Parameters:**

- `volume_id` (string): Volume ID or name

**Returns:**

- `bool`: True if successfully started, raises error on failure

**Example:**

```python
import knot.volume

# Start a volume
if knot.volume.start("data-volume"):
    print("Volume started successfully")
```

---

### stop(volume_id)

Stop a running volume.

**Parameters:**

- `volume_id` (string): Volume ID or name

**Returns:**

- `bool`: True if successfully stopped, raises error on failure

**Example:**

```python
import knot.volume

# Stop a volume
if knot.volume.stop("data-volume"):
    print("Volume stopped successfully")
```

---

### is_running(volume_id)

Check if a volume is currently running.

**Parameters:**

- `volume_id` (string): Volume ID or name

**Returns:**

- `bool`: True if the volume is running, False otherwise

**Example:**

```python
import knot.volume

# Check volume status
if knot.volume.is_running("data-volume"):
    print("Volume is running")
else:
    print("Volume is stopped")

# Start if not running
if not knot.volume.is_running("data-volume"):
    knot.volume.start("data-volume")
```

---

## Usage Examples

### Example 1: Setting Up Storage Volumes

```python
import knot.volume

def setup_storage_volumes():
    """Create standard storage volumes"""

    volumes = [
        {
            "name": "database-storage",
            "definition": "size: 100GB, type: ssd",
            "platform": "linux/amd64",
        },
        {
            "name": "app-data",
            "definition": "size: 50GB, type: hdd",
            "platform": "linux/amd64",
        },
        {
            "name": "backup-storage",
            "definition": "size: 500GB, type: hdd",
            "platform": "linux/amd64",
        },
        {
            "name": "cache-volume",
            "definition": "size: 10GB, type: nvme",
            "platform": "linux/amd64",
        },
    ]

    created = []
    for vol in volumes:
        # Check if volume exists
        existing = knot.volume.list()
        if any(v['name'] == vol['name'] for v in existing):
            print(f"Volume '{vol['name']}' already exists")
        else:
            volume_id = knot.volume.create(**vol)
            print(f"Created volume: {vol['name']} ({volume_id})")
            created.append(volume_id)

    return created

setup_storage_volumes()
```

### Example 2: Volume Lifecycle Management

```python
import knot.volume

def manage_volume_lifecycle(volume_name):
    """Complete volume lifecycle management"""

    # Create volume
    volume_id = knot.volume.create(
        name=volume_name,
        definition="size: 20GB",
        platform="linux/amd64"
    )
    print(f"Created volume: {volume_id}")

    # Start the volume
    if not knot.volume.is_running(volume_id):
        knot.volume.start(volume_id)
        print("Volume started")

    # Get volume details
    vol = knot.volume.get(volume_id)
    print(f"Volume: {vol['name']}")
    print(f"Active: {vol['active']}")
    print(f"Definition: {vol['definition']}")

    # Update volume
    knot.volume.update(
        volume_id,
        definition="size: 30GB, type: ssd"
    )
    print("Volume updated")

    # Stop volume when done
    if knot.volume.is_running(volume_id):
        knot.volume.stop(volume_id)
        print("Volume stopped")

    return volume_id

# manage_volume_lifecycle("test-volume")
```

### Example 3: Volume Monitoring

```python
import knot.volume

def monitor_volumes():
    """Monitor all volumes and their status"""

    volumes = knot.volume.list()

    print(f"{'Name':<25} {'Zone':<15} {'Platform':<15} {'Status':<10}")
    print("-" * 65)

    running = 0
    stopped = 0

    for vol in volumes:
        status = "Running" if vol['active'] else "Stopped"
        print(f"{vol['name']:<25} {vol['zone']:<15} "
              f"{vol['platform']:<15} {status:<10}")

        if vol['active']:
            running += 1
        else:
            stopped += 1

    print("-" * 65)
    print(f"Total: {len(volumes)} (Running: {running}, Stopped: {stopped})")

    # Find stopped volumes
    stopped_volumes = [v for v in volumes if not v['active']]
    if stopped_volumes:
        print(f"\nStopped volumes: {len(stopped_volumes)}")
        for vol in stopped_volumes:
            print(f"  - {vol['name']}")

monitor_volumes()
```

### Example 4: Volume Maintenance

```python
import knot.volume

def start_all_volumes():
    """Start all stopped volumes"""

    volumes = knot.volume.list()
    stopped = [v for v in volumes if not v['active']]

    if not stopped:
        print("All volumes are already running")
        return

    print(f"Starting {len(stopped)} volumes...")
    for vol in stopped:
        try:
            knot.volume.start(vol['id'])
            print(f"  Started: {vol['name']}")
        except Exception as e:
            print(f"  Failed to start {vol['name']}: {e}")

def stop_all_volumes():
    """Stop all running volumes"""

    volumes = knot.volume.list()
    running = [v for v in volumes if v['active']]

    if not running:
        print("All volumes are already stopped")
        return

    print(f"Stopping {len(running)} volumes...")
    for vol in running:
        try:
            knot.volume.stop(vol['id'])
            print(f"  Stopped: {vol['name']}")
        except Exception as e:
            print(f"  Failed to stop {vol['name']}: {e}")

# Usage
# start_all_volumes()
# stop_all_volumes()
```

### Example 5: Volume Backup Strategy

```python
import knot.volume

def setup_backup_volumes():
    """Set up volumes with backup strategy"""

    # Primary volumes
    primary_volumes = [
        ("database-primary", "size: 100GB, type: ssd"),
        ("app-data-primary", "size: 50GB, type: ssd"),
    ]

    # Backup volumes
    backup_volumes = [
        ("database-backup", "size: 100GB, type: hdd"),
        ("app-data-backup", "size: 50GB, type: hdd"),
    ]

    # Create primary volumes
    print("Creating primary volumes:")
    for name, definition in primary_volumes:
        existing = knot.volume.list()
        if any(v['name'] == name for v in existing):
            print(f"  {name} already exists")
        else:
            vol_id = knot.volume.create(name, definition)
            knot.volume.start(vol_id)
            print(f"  Created and started: {name}")

    # Create backup volumes
    print("\nCreating backup volumes:")
    for name, definition in backup_volumes:
        existing = knot.volume.list()
        if any(v['name'] == name for v in existing):
            print(f"  {name} already exists")
        else:
            vol_id = knot.volume.create(name, definition)
            # Keep backups stopped until needed
            print(f"  Created: {name} (stopped)")

setup_backup_volumes()
```

---

## Notes

### Volume Definition Format

The `definition` parameter specifies volume properties:
- Basic: `"size: 10GB"`
- With type: `"size: 50GB, type: ssd"`
- Advanced: `"size: 100GB, type: nvme, iops: 5000"`

Common types:
- `hdd`: Hard disk drive (cheaper, slower)
- `ssd`: Solid state drive (faster)
- `nvme`: NVMe storage (fastest)

### Volume States

- **Running (active=true)**: Volume is online and accessible
- **Stopped (active=false)**: Volume is offline but not deleted

### Volume Deletion

When you delete a volume:
- All data on the volume is permanently deleted
- Spaces using the volume may lose access
- The volume cannot be recovered

### Best Practices

1. **Stop volumes before deletion** to ensure clean shutdown
2. **Use appropriate volume types** based on performance needs
3. **Monitor volume status** to ensure availability
4. **Plan for backups** using separate backup volumes
5. **Label volumes clearly** with descriptive names

### Volume vs Space Storage

- **Volumes**: Persistent storage managed separately from spaces
- **Space storage**: Ephemeral storage tied to space lifecycle
- Use volumes for data that must persist across space recreations

---

## Related Libraries

- **knot.space** - For spaces that can use volumes
- **knot.template** - For templates that can specify volumes
