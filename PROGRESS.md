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

 - Memory limits (cgroup v2)
 - CPU limits (cgroup v2)
 - Process count limits
 - Cgroup lifecycle management
 - Resource statistics tracking

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

 - Container metadata persistence (JSON)
 - State tracking (created, running, stopped, exited)
 - Container ID generation
 - PID tracking and management
 - Log file management

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

 - Partial container ID matching (Docker-like)
 - Background container execution (detached mode)
 - Goroutine-based process monitoring
 - Automatic cleanup on container exit
 - Process namespace entry with nsenter

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

## ✅ Phase 2C: Container Networking (COMPLETED)

### Overview
This phase implemented **network namespace isolation**, **virtual Ethernet (veth) pairs**, and a **software bridge (`minidocker0`)** to give each container its own virtual network interface and Internet access — similar to Docker’s default `bridge` mode.

### Features Implemented

- [x] **Bridge Creation** (`minidocker0`)
  - Automatically creates a Linux bridge if missing
  - Assigns IP `172.18.0.1/24`
  - Enables IP forwarding (`sysctl net.ipv4.ip_forward=1`)
  - Configures NAT masquerading for outbound traffic

- [x] **Container Network Setup**
  - Creates a veth pair (`vethXXXX` ↔ `vethcXXXX`)
  - Attaches the host end to `minidocker0`
  - Moves container end to the container’s network namespace
  - Renames container-side interface to `eth0`
  - Allocates random IPs within `172.18.0.0/24`
  - Sets up default route `via 172.18.0.1`
  - Brings up `lo` and `eth0` inside container

- [x] **NAT & Internet Connectivity**
  - Outbound NAT via the host’s external interface (e.g., `enp0s1`)
  - MASQUERADE rule:
    ```bash
    iptables -t nat -A POSTROUTING -s 172.18.0.0/24 -o enp0s1 -j MASQUERADE
    ```
  - Containers can successfully reach the Internet (e.g., `ping 8.8.8.8`)

- [x] **Network Cleanup**
  - Automatic deletion of veth pairs on container stop or removal
  - Removal of `/var/run/netns/minidocker-<pid>` symlinks
  - Bridge persists across containers, reused as needed

### Debugging Journey & Key Fixes

1. **Host Lost Internet After Bridge Creation**
   - Initial NAT rule used `! -o minidocker0`, which caused host-local packets to be NATed incorrectly.
   - **Fix:** Replaced with explicit external interface NAT rule (`-o enp0s1`).

2. **Bridge Down / No Carrier**
   - The bridge initially appeared as `DOWN` with no attached veths.
   - Once veth pairs were correctly attached, state changed to `UP`.

3. **Container Could Not Reach Internet**
   - Root cause: `allocateIP()` generated addresses in `172.10.x.x`, outside the bridge’s `172.18.x.x` subnet.
   - **Fix:** Updated allocator to use `172.18.0.0/24`.

4. **Invalid Gateway Error**
   - “`Nexthop has invalid gateway`” appeared during route setup.
   - Caused by mismatched IP subnet (`172.10.x.x` vs `172.18.x.x`).
   - **Fix:** Same as above — corrected IP allocation and CIDR validation.

5. **Network Namespace Not Isolated**
   - Containers initially shared host network namespace (`inode` IDs identical).
   - **Root Cause:** `SysProcAttr.Cloneflags` was never set with `CLONE_NEWNET`.
   - **Fix:** Added:
     ```go
     cmd.SysProcAttr = &syscall.SysProcAttr{
         Cloneflags: syscall.CLONE_NEWUTS |
                     syscall.CLONE_NEWPID |
                     syscall.CLONE_NEWNS  |
                     syscall.CLONE_NEWNET,
     }
     ```
   - Verified by checking differing inode numbers between host and container:
     ```bash
     sudo stat -Lc '%i' /proc/<pid>/ns/net
     sudo stat -Lc '%i' /proc/self/ns/net
     # Different => isolated
     ```

6. **Verification via nsenter**
   - Confirmed isolation and connectivity:
     ```bash
     sudo nsenter --target <pid> --net ip addr show   # lo + eth0 (172.18.0.X)
     sudo nsenter --target <pid> --net ip route show  # default via 172.18.0.1
     sudo nsenter --target <pid> --net ping -c 3 8.8.8.8
     ```
   - Pings succeeded, full bridge + NAT path verified.

7. **Persistent Routing & Cleanup**
   - Ensured bridge routes remain in kernel routing table:
     ```
     172.18.0.0/24 dev minidocker0 proto kernel scope link src 172.18.0.1
     ```
   - NAT and forward rules appended only once, checked for duplicates.

### Final Verification Results

| Check | Expected | Result |
|-------|-----------|--------|
| `minidocker0` bridge created | ✅ Exists, UP | ✅ |
| veth pair creation | ✅ host ↔ container | ✅ |
| container network namespace | ✅ Isolated (unique inode) | ✅ |
| container IP assignment | ✅ 172.18.0.X/24 | ✅ |
| default route | ✅ via 172.18.0.1 | ✅ |
| Internet connectivity | ✅ ping 8.8.8.8 works | ✅ |
| Host Internet | ✅ unaffected | ✅ |
| NAT rule | ✅ -o enp0s1 | ✅ |
| Cleanup | ✅ removes veth and symlinks | ✅ |

### Key Learnings

- Always verify namespace isolation via inode comparison.
- `CLONE_NEWNET` must be explicitly set in `SysProcAttr.Cloneflags`.
- NAT should target the actual external interface, not `! -o bridge`.
- Gateway must lie inside container’s subnet, or Linux rejects route.
- Bridge doesn’t go `UP` until at least one active veth is attached.
- Network namespaces can be debugged safely from host with `nsenter`.
- Each container now behaves like a lightweight virtual machine with its own `eth0`, IP, and routing table.

---

### Current Capabilities

- Containers have **isolated networking stacks**
- Each container gets its own **veth + IP**
- Internet access via **NAT through host**
- Clean resource teardown
- Fully automated bridge lifecycle
- Works identically to Docker’s default `bridge` mode

---

**Next Steps (Upcoming Phase 3)**
- DNS resolution inside containers (`/etc/resolv.conf` management)
- Support for custom user-defined bridges
- Option for `--net=none` or `--net=host` modes
- Inter-container communication on same bridge
- Persistent IP assignment (IP map tracking)

