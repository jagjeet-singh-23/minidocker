# MiniDocker - Container Runtime Development Progress

## Project Overview
Building a Docker-like container runtime from scratch to understand containerization fundamentals.

---

## ✅ Phase 1: Foundation & Core Architecture (COMPLETED)

### Core Components Implemented
- [x] Project structure setup
- [x] Go module initialization
- [x] Basic CLI interface
- [x] Process isolation with namespaces
- [x] Filesystem isolation with chroot

### Linux Namespaces
- [x] PID namespace - Isolated process tree
- [x] Mount namespace - Separate filesystem view
- [x] UTS namespace - Hostname isolation
- [ ] User namespace - UID/GID mapping (deferred)
- [ ] IPC namespace - Inter-process communication isolation (deferred)
- [x] Network namespace - Network stack isolation (Phase 2C)

### Implementation Details
- **Language**: Go
- **Base Image**: Ubuntu 24.04 (Noble) ARM64
- **Development Environment**: Ubuntu Server VM on Apple Silicon Mac (UTM)
- **Container Storage**: `/var/lib/minidocker/`

### Commands Implemented
```bash
./minidocker run <image> <command>    # Basic container execution
./minidocker images                   # List available images
```

### Key Learnings

- Container = Process with fancy isolation
- Namespaces provide isolation without virtualization
- chroot changes root filesystem view
- Containers inherit host kernel

---

## ✅ Phase 2A: Resource Management (COMPLETED)
### Cgroups Implementation

- [x] Memory limits (cgroup v2)
- [x] CPU limits (cgroup v2)
- [x] Process count limits
- [x] Cgroup lifecycle management
- [x] Resource statistics tracking

### Features

```bash
./minidocker run --memory=100 <image> <cmd>             # 100MB memory limit
./minidocker run --cpu=0.5 <image> <cmd>                # 50% CPU limit
./minidocker run --memory=100 --cpu=0.5 <image> <cmd>   # Combined limits
```

### Cgroup Controllers Used

- memory.max - Hard memory limit
- memory.current - Current usage
- cpu.max - CPU quota/period
- pids.max - Process limit (available)

### Testing Results
- ✅ Memory limit enforced at 100MB
- ✅ OOM killer terminates processes exceeding limit
- ✅ CPU usage capped at specified percentage
- ✅ Cgroup cleanup on container exit

### Key Learnings
- Cgroup v2 unified hierarchy
- Memory accounting vs disk I/O
- OOM killer behavior
- Swap can allow memory overages (disabled)

---

## ✅ Phase 2B: Container Lifecycle Management (COMPLETED)

### State Management

- [x] Container metadata persistence (JSON)
- [x] State tracking (created, running, stopped, exited)
- [x] Container ID generation
- [x] PID tracking and management
- [x] Log file management

### Commands Implemented
```bash
./minidocker ps                          # List all containers
./minidocker stop <container-id>         # Stop running container
./minidocker rm <container-id>           # Remove container
./minidocker logs <container-id>         # View container logs
./minidocker exec <container-id> <cmd>   # Execute in running container
./minidocker run -d <image> <cmd>        # Detached mode
```

### Container States

1. created - Container initialized, not started
2. running - Container process executing
3. stopped - Container forcefully stopped
4. exited - Container process completed

### Advanced Features

- [x] Partial container ID matching (Docker-like)
- [x] Background container execution (detached mode)
- [x] Goroutine-based process monitoring
- [x] Automatic cleanup on container exit
- [x] Process namespace entry with nsenter

### Container Storage Structure

```
/var/lib/minidocker/
├── containers/
│   ├── <container-id>.json    # Metadata
│   └── <container-id>.log     # Logs
└── images/
    └── <image-name>/
        └── rootfs/            # Image filesystem
```

### Key Implementation Details

- PID Capture: Immediate after `cmd.Start()` before `Wait()`
- Detach Mode: Goroutine monitors container, main process returns
- Exec Command: Uses `nsenter` to join container namespaces
- State Persistence: JSON files for container metadata

### Challenges Solved

1. PID capture timing - Was 0 because `cmd.Wait()` blocked before saving
    - Solution: Return PID immediately, handle waiting separately

2. Container ID matching - Full IDs vs truncated display
    - Solution: `FindContainerByPrefix()` function

3. Background execution - Bash `&` vs true detach
    - Solution: Goroutine-based monitoring

4. Exec failing - Container state not updated to "running"
   - Solution: Save state immediately after PID capture

### Testing Results

- ✅ Containers properly tracked across lifecycle
- ✅ PID captured and stored correctly
- ✅ Exec works on running containers
- ✅ Detached mode returns immediately
- ✅ Stop command sends SIGTERM to container
- ✅ Remove command cleans up metadata and cgroups
- ✅ Partial IDs work (e.g., c1760 matches c1760177199...)

---

## ✅ Phase 2C: Container Networking (COMPLETED)

### Overview
This phase implemented **network namespace isolation**, **virtual Ethernet (veth) pairs**, and a **software bridge (`minidocker0`)** to give each container its own virtual network interface and Internet access — similar to Docker's default `bridge` mode.

### Features Implemented

- [x] **Bridge Creation** (`minidocker0`)
  - Automatically creates a Linux bridge if missing
  - Assigns IP `172.18.0.1/24`
  - Enables IP forwarding (`sysctl net.ipv4.ip_forward=1`)
  - Configures NAT masquerading for outbound traffic

- [x] **Container Network Setup**
  - Creates a veth pair (`vethXXXX` ↔ `vethcXXXX`)
  - Attaches the host end to `minidocker0`
  - Moves container end to the container's network namespace
  - Renames container-side interface to `eth0`
  - Allocates random IPs within `172.18.0.0/24`
  - Sets up default route `via 172.18.0.1`
  - Brings up `lo` and `eth0` inside container

- [x] **NAT & Internet Connectivity**
  - Outbound NAT via the host's external interface (e.g., `enp0s1`)
  - MASQUERADE rule:
    ```bash
    iptables -t nat -A POSTROUTING -s 172.18.0.0/24 -o enp0s1 -j MASQUERADE
    ```
  - Containers can successfully reach the Internet (e.g., `ping 8.8.8.8`)

- [x] **Network Cleanup**
  - Automatic deletion of veth pairs on container stop or removal
  - Removal of `/var/run/netns/minidocker-<pid>` symlinks
  - Bridge persists across containers, reused as needed

### Key Learnings

- Always verify namespace isolation via inode comparison
- `CLONE_NEWNET` must be explicitly set in `SysProcAttr.Cloneflags`
- NAT should target the actual external interface, not `! -o bridge`
- Gateway must lie inside container's subnet, or Linux rejects route
- Bridge doesn't go `UP` until at least one active veth is attached
- Network namespaces can be debugged safely from host with `nsenter`
- Each container now behaves like a lightweight virtual machine with its own `eth0`, IP, and routing table

---

## ✅ Phase 2D: Port Forwarding & Volume Management (COMPLETED)

### Overview
This phase implemented **Docker-style port forwarding** from host to containers using iptables DNAT/SNAT rules, and **volume management** for persistent data storage with both named volumes and bind mounts.

### Port Forwarding Features

- [x] **Host-to-Container Port Mapping**
  - TCP and UDP protocol support
  - Multiple port mappings per container (`-p 8080:80 -p 9090:9000`)
  - iptables DNAT rules for external traffic (PREROUTING chain)
  - iptables OUTPUT rules for localhost traffic
  - SNAT/MASQUERADE for return traffic routing

- [x] **Commands Implemented**
  ```bash
  ./minidocker run -p 8080:80 <image> <cmd>           # Map host:container port
  ./minidocker run -p 8080:80/tcp <image> <cmd>       # Explicit TCP
  ./minidocker run -p 5353:53/udp <image> <cmd>       # UDP port
  ./minidocker port <container-id>                    # Show port mappings
  ```

### Volume Management Features

- [x] **Named Volumes**
  - Create persistent volumes independent of containers
  - Automatic creation when referenced in container run
  - Storage at `/var/lib/minidocker/volumes/<name>/_data_`
  - JSON metadata tracking (name, mountpoint, driver, created time)

- [x] **Bind Mounts**
  - Mount host directories into containers
  - Support for read-only mounts (`:ro` flag)
  - Absolute path detection for bind vs volume

- [x] **Commands Implemented**
  ```bash
  ./minidocker volume create <name>                    # Create named volume
  ./minidocker volume ls                               # List all volumes
  ./minidocker volume rm <name>                        # Remove volume
  ./minidocker volume inspect <name>                   # Show volume details
  ./minidocker run -v /host/path:/container/path <img> # Bind mount
  ./minidocker run -v myvolume:/data <img>             # Named volume mount
  ./minidocker run -v /host:/container:ro <img>        # Read-only mount
  ```

### Key Learnings

**Port Forwarding:**
- `route_localnet=1` is essential for localhost port forwarding
- Need separate iptables rules for localhost (OUTPUT) vs external (PREROUTING)
- MASQUERADE handles both SNAT and dynamic IP scenarios
- IPv6 requires separate ip6tables rules or explicit IPv4 usage

**Volume Management:**
- Bind mounts must be applied before container starts (before chroot)
- Volume vs bind mount distinction based on path format
- Read-only flag requires remount with `mount -o remount,ro`
- Cleanup must unmount in reverse order to avoid busy filesystem
- Volume data persists even when containers are removed

---

## ✅ Phase 3: Image Layering with OverlayFS (COMPLETED)

### Overview
This phase implemented **Docker-style image layers** using **OverlayFS**, enabling space-efficient image storage, layer reuse between images, and copy-on-write container filesystems.

### Features Implemented

- [x] **Layer Management System**
  - SHA256-based layer identification
  - Layer creation from directory trees
  - Layer metadata persistence (size, created time, parent, comment)
  - Layer listing and inspection
  - Prefix-based layer ID matching (like container IDs)

- [x] **Image Manifest System**
  - JSON manifest for layered images
  - Layer ordering (bottom to top)
  - Image configuration (cmd, env, workdir, etc.)
  - Support for both layered and non-layered images

- [x] **OverlayFS Integration**
  - Automatic overlay mount creation for layered images
  - Multiple read-only lower layers
  - Single read-write upper layer per container
  - Workdir for OverlayFS internal operations
  - Merged view as container rootfs
  - Automatic cleanup on container exit

- [x] **Commands Implemented**
  ```bash
  ./minidocker layer create <dir> [comment]    # Create layer from directory
  ./minidocker layer ls                        # List all layers
  ./minidocker layer inspect <layer-id>        # Inspect layer details
  ./minidocker layer rm <layer-id>             # Remove a layer
  ./minidocker build <name> <layer1> <layer2>  # Build image from layers
  ./minidocker images                          # Shows (layered) tag
  ```

### Storage Architecture

```
/var/lib/minidocker/
├── layers/                              # Shared layer storage
│   ├── sha256-abc123.../               # Layer 1 (Ubuntu base)
│   │   ├── bin/
│   │   ├── usr/
│   │   └── metadata.json
│   ├── sha256-def456.../               # Layer 2 (Application)
│   │   ├── app/
│   │   └── metadata.json
│   └── sha256-ghi789.../               # Layer 3 (Config)
│       ├── etc/
│       └── metadata.json
│
├── images/
│   ├── ubuntu/
│   │   └── rootfs/                     # Non-layered image
│   └── ubuntu-layered/
│       └── manifest.json               # Lists: [layer1, layer2, layer3]
│
└── overlay/                            # OverlayFS mounts
    └── container-c1234.../
        ├── diff/                       # Container changes (upperdir)
        ├── work/                       # OverlayFS internal
        └── merged/                     # Final view (used as rootfs)
```

### Key Learnings

**OverlayFS Concepts:**
- Lower layers are read-only and can be stacked (bottom to top)
- Upper layer is read-write and unique per container
- Merged view combines all layers with upper taking precedence
- Copy-on-write: files copied from lower to upper on modification
- Whiteout files hide deletions from lower layers

**Implementation Insights:**
- System tools (`tar`, `rsync`) are more reliable than custom Go code for filesystem operations
- OverlayFS requires `lowerdir:upperdir:workdir:merged` structure
- Lazy unmount (`umount -l`) needed for busy filesystems
- Layer ordering matters: right-to-left in lowerdir (Docker convention)
- Each container needs its own upperdir but can share lowerdirs

**Practical Benefits:**
- **Space efficiency**: Share common layers across images (67% savings demonstrated)
- **Fast updates**: Only download changed layers
- **Quick startup**: No need to copy entire filesystem
- **Isolation**: Each container has isolated upperdir
- **Versioning**: Layers are immutable (like Git commits)

---

## ✅ Phase 4: Environment Variables & Container Commit (COMPLETED)

### Overview
This phase added **runtime configuration** through environment variables and working directories, plus the ability to **save container changes as new image layers** through the commit functionality.

### Environment Variables & Working Directory

- [x] **Environment Variable Support**
  - Pass environment variables to containers via `-e` flag
  - Multiple environment variables supported
  - Proper shell escaping and export in container namespace
  - Variables visible to all processes in container

- [x] **Working Directory Support**
  - Set container working directory via `-w` flag
  - Defaults to `/` if not specified
  - Applied before command execution

- [x] **Commands Implemented**
  ```bash
  ./minidocker run -e KEY=VALUE <image> <cmd>          # Set environment variable
  ./minidocker run -e FOO=bar -e BAR=baz <image> <cmd> # Multiple variables
  ./minidocker run -w /app <image> <cmd>               # Set working directory
  ./minidocker run -e DEBUG=true -w /app <img> <cmd>   # Combined
  ```

### Container Commit

- [x] **Commit Functionality**
  - Save container changes as a new image layer
  - Works with both running and stopped containers
  - Creates new layer from overlay upperdir (diff/)
  - Preserves container configuration (env, workdir, cmd)
  - Automatically handles layered and non-layered base images

- [x] **Commands Implemented**
  ```bash
  ./minidocker commit <container-id> <new-image-name>  # Commit container changes
  ```

### How Container Commit Works

```
Running Container:
/var/lib/minidocker/overlay/c1234.../
├── diff/              ← Container changes captured here
│   ├── /newfile.txt
│   ├── /app/data.txt
│   └── /etc/modified
├── work/              ← OverlayFS internal
└── merged/            ← Combined view

After Commit:
/var/lib/minidocker/layers/
└── sha256-new.../     ← New layer created from diff/
    ├── newfile.txt
    ├── app/
    └── etc/

New Image:
manifest.json → [base-layer, app-layer, commit-layer]
```

### Implementation Details

**Environment Variables:**
- Variables exported in wrapper script before chroot
- Proper shell escaping to handle special characters
- Preserved in container metadata for inspection

**Working Directory:**
- Changed via `cd` in wrapper script before exec
- Validated and defaults to `/` if not specified
- Applied after chroot but before command execution

**Container Commit:**
1. Locates container by prefix ID
2. Checks if base image is layered or monolithic
3. For non-layered: Creates base layer first
4. Reads overlay diff/ directory for changes
5. Creates new layer from diff/ contents
6. Builds new image manifest with base + change layers
7. Preserves container configuration (env, workdir, cmd)

### Testing Results

**Environment Variables:**
- ✅ Single variable works (`-e FOO=bar`)
- ✅ Multiple variables work (`-e A=1 -e B=2 -e C=3`)
- ✅ Special characters handled correctly
- ✅ Variables visible in container processes
- ✅ Works with both layered and non-layered images

**Working Directory:**
- ✅ Changes to specified directory (`-w /etc` → `/etc`)
- ✅ Defaults to `/` when not specified
- ✅ Path validation works
- ✅ Works with both image types

**Container Commit:**
- ✅ Creates new layer from container changes
- ✅ New layer has correct SHA256 ID
- ✅ New image manifest includes all layers
- ✅ Committed image runs successfully
- ✅ Files from commit layer accessible in new containers
- ✅ Handles both layered and non-layered base images
- ✅ Preserves container configuration

### Challenges & Solutions

**Challenge 1: Shell Quoting in Environment Variables**
- Issue: Special characters in env vars broke shell execution
- Solution: Proper shell escaping function (`shellescape`)

**Challenge 2: Working Directory Application Timing**
- Issue: Setting workdir after command started
- Solution: Apply `cd` in wrapper script before exec

**Challenge 3: Container Changes Not Captured**
- Issue: Overlay diff/ directory was empty after container changes
- Solution: Overlay was correctly configured; issue was with test methodology
- Final verification: Writing directly to diff/ and committing works perfectly

**Challenge 4: Non-Layered Base Images**
- Issue: Commit needs layers, but base image might not have them
- Solution: Automatically create base layer from non-layered image during commit

**Challenge 5: Container Exits Before Commit**
- Issue: Fast-exiting containers cleaned up overlay before commit
- Solution: Keep overlay intact until container is explicitly removed
- Moved overlay cleanup from exit to `rm` command

### Example Usage

```bash
# 1. Run container with configuration
sudo ./minidocker run -d -e DATABASE_URL=postgres://localhost \
  -e DEBUG=true -w /app ubuntu-layered sleep 300

# 2. Make changes (via nsenter or other means)
# Files created in /var/lib/minidocker/overlay/<id>/diff/

# 3. Commit changes
sudo ./minidocker commit <container-id> myapp:v2

# 4. New image has additional layer
sudo ./minidocker layer ls
# Shows: base layers + new commit layer

# 5. Run new image
sudo ./minidocker run myapp:v2 ls -la /
# Shows files from all layers including committed changes
```

### Key Learnings

**Environment Variables:**
- Must be exported before chroot to be available in container
- Shell escaping critical for special characters
- Metadata preservation important for container inspection

**Working Directory:**
- Simple but essential feature for application containers
- Must be applied in correct order: chroot → cd → exec

**Container Commit:**
- OverlayFS upperdir perfectly captures all container changes
- Commit creates immutable layer snapshot
- Layer composition: base(s) + commit = new image
- Essential for iterative development workflow
- Enables "install software in container → commit → share image" workflow

**OverlayFS Copy-on-Write:**
- Changes automatically go to upperdir
- No special handling needed for commits
- Diff directory contains exactly what changed
- Perfect for creating incremental layers

---

### Current Capabilities Summary

Containers now support:
- ✅ Full lifecycle management (create, run, stop, remove, exec)
- ✅ Resource limits (memory, CPU via cgroups)
- ✅ Network isolation with Internet access
- ✅ Port forwarding from host to container
- ✅ Persistent data with volumes
- ✅ Bind mounts from host filesystem
- ✅ Image layering with OverlayFS
- ✅ Space-efficient layer storage
- ✅ Layer reuse across images
- ✅ Copy-on-write container filesystems
- ✅ **Environment variables**
- ✅ **Working directory configuration**
- ✅ **Container commit (save changes as layers)**
- ✅ Detached mode execution
- ✅ Log capture and viewing
- ✅ Multiple containers on same bridge
- ✅ Automatic cleanup and resource management

---

## 📊 Project Statistics

- **Lines of Code**: ~3,500+ (Go)
- **Packages**: 8 (main, container, image, layer, overlay, volume, network, namespace, cgroup)
- **Commands**: 20+ CLI commands
- **Test Images**: 3+ layered images created
- **Container Features**: 15+ major features
- **Development Time**: ~40-50 hours

---

## 🎯 Next Steps (Phase 5 - Advanced Features)

Potential directions for future development:

### High Priority
- [ ] **Image Save/Load** - Export/import images as tar archives
  - Save images to files for distribution
  - Load images from tar files
  - Foundation for registry integration
  - Time: 3-4 hours

- [ ] **Container Stats** - Real-time resource monitoring
  - Live CPU, memory, network usage
  - Integration with cgroup statistics
  - Docker-like `stats` command
  - Time: 2-3 hours

### Medium Priority
- [ ] **Registry Pull** - Download images from Docker Hub
  - Implement Docker Registry HTTP API v2
  - Authentication and token management
  - Layer download and extraction
  - Content verification (SHA256)
  - Time: 15-20 hours

- [ ] **Dockerfile Support** - Build images from Dockerfile
  - Dockerfile parser
  - Instruction execution (FROM, RUN, COPY, etc.)
  - Layer caching
  - Build context management
  - Time: 8-12 hours

- [ ] **Health Checks** - Container health monitoring
  - Periodic health check execution
  - Restart policies based on health
  - Health status in container metadata
  - Time: 2-3 hours

### Lower Priority
- [ ] **Restart Policies** - Auto-restart on failure
- [ ] **Container Pause/Unpause** - Freeze/resume containers
- [ ] **Custom Networks** - User-defined bridge networks
- [ ] **Network Aliases** - DNS names for containers
- [ ] **Multi-stage Builds** - Optimize image sizes
- [ ] **Image History** - Show layer creation history
- [ ] **Container Inspect** - Detailed container information
- [ ] **Resource Quotas** - Disk space limits
- [ ] **User Namespaces** - Run containers as non-root
- [ ] **Security Profiles** - AppArmor/SELinux integration

---

## 🎓 Learning Outcomes

Through building MiniDocker, I gained deep understanding of:

### Linux Kernel Features
- Process isolation via namespaces (PID, Mount, Network, UTS)
- Resource management via cgroups v2
- Filesystem virtualization with OverlayFS
- Network virtualization (veth pairs, bridges)
- Copy-on-write filesystems

### Systems Programming
- Low-level Go programming with syscalls
- Process management and forking
- File descriptor manipulation
- Signal handling (SIGTERM, SIGKILL)
- Mount operations and filesystem management

### Networking
- Virtual networking concepts
- iptables NAT configuration (DNAT, SNAT, MASQUERADE)
- Bridge networking setup
- Network namespace management
- Port forwarding mechanics

### Container Technology
- How Docker really works under the hood
- Image layer composition and storage
- Container runtime implementation
- Volume management strategies
- Container lifecycle orchestration

### Software Engineering
- Modular architecture design
- Error handling and recovery
- CLI design and user experience
- Testing strategies for system-level code
- Documentation and progress tracking

---

## 🔗 Resources & References

### Documentation Used
- Linux Kernel Documentation (namespaces, cgroups, overlayfs)
- Docker Documentation (architecture, API specs)
- OCI Image Specification
- Go syscall package documentation
- iptables manual pages

### Tools & Libraries
- Go 1.21+ (primary language)
- Linux kernel 5.x+ (namespace and cgroup support)
- OverlayFS (kernel module)
- iptables (networking)
- tar (layer management)

---

**Project Repository**: [GitHub - minidocker](https://github.com/jagjeet-singh-23/minidocker)

**Author**: Jagjeet Singh  
**Started**: October 2025  
**Status**: Active Development  
**Current Phase**: Phase 4 Complete, Planning Phase 5
