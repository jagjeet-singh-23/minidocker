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

### Debugging Journey & Key Fixes

1. **Host Lost Internet After Bridge Creation**
   - Initial NAT rule used `! -o minidocker0`, which caused host-local packets to be NATed incorrectly.
   - **Fix:** Replaced with explicit external interface NAT rule (`-o enp0s1`).

2. **Bridge Down / No Carrier**
   - The bridge initially appeared as `DOWN` with no attached veths.
   - Once veth pairs were correctly attached, state changed to `UP`.

3. **Container Could Not Reach Internet**
   - Root cause: `allocateIP()` generated addresses in `172.10.x.x`, outside the bridge's `172.18.x.x` subnet.
   - **Fix:** Updated allocator to use `172.18.0.0/24`.

4. **Invalid Gateway Error**
   - "`Nexthop has invalid gateway`" appeared during route setup.
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
- Gateway must lie inside container's subnet, or Linux rejects route.
- Bridge doesn't go `UP` until at least one active veth is attached.
- Network namespaces can be debugged safely from host with `nsenter`.
- Each container now behaves like a lightweight virtual machine with its own `eth0`, IP, and routing table.

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

- [x] **iptables Rules Configuration**
  - PREROUTING: DNAT for external connections
  - OUTPUT: DNAT for localhost connections
  - POSTROUTING: MASQUERADE for source NAT
  - FORWARD: Allow forwarded traffic to/from container

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

- [x] **Mount Types**
  - **bind**: Direct host path mapping
  - **volume**: Named volume from minidocker storage
  - Automatic type detection based on path (absolute = bind, name = volume)

### Debugging Journey & Key Fixes

1. **Localhost Port Forwarding Not Working**
   - Initial implementation only had PREROUTING rules for external traffic
   - Localhost connections (`127.0.0.1:8080`) weren't being routed
   - **Fix:** Added OUTPUT chain DNAT rules for localhost traffic
   - Required kernel parameter: `net.ipv4.conf.all.route_localnet=1`

2. **IPv6 Interfering with Tests**
   - `curl http://localhost:8080` used IPv6 (`::1`) instead of IPv4
   - iptables rules were IPv4-only
   - **Fix:** Use explicit IPv4 address `curl http://127.0.0.1:8080`
   - Alternative: Disable IPv6 on loopback or add ip6tables rules

3. **Connection Reset by Peer**
   - Netcat (`nc -l -p 80`) closes connection immediately after sending response
   - This is normal behavior for simple netcat servers
   - **Fix:** Use `nc -l -p 80 -q 1` to wait before closing, or use Python HTTP server
   - Verified with tcpdump: HTTP response was sent correctly before RST

4. **Variable Shadowing Bug**
   - Port forwarding rules weren't being applied
   - Root cause: `containerIP, err := network.SetupContainerNetwork()` shadowed outer variable
   - **Fix:** Changed to `ip, err := ...` and assigned to outer `containerIP` variable
   - This bug prevented IP from being passed to `SetupPortForwarding()`

5. **SNAT Rules for Return Traffic**
   - Initial implementation only had DNAT, return packets weren't routed correctly
   - **Fix:** Added MASQUERADE rules in POSTROUTING chain for:
     - Localhost-originated traffic to container
     - Container responses back to localhost
   - Also added reverse FORWARD rules for established connections

6. **Multiple Volume Mounts**
   - Need to support multiple `-v` flags like Docker
   - **Fix:** Implemented custom `arrayFlags` type satisfying `flag.Value` interface
   - Allows repeated flags: `-v /host1:/data1 -v /host2:/data2`

7. **Mount Timing Issues**
   - Mounts must be applied to rootfs BEFORE container starts
   - After container is chrooted, host paths are no longer accessible
   - **Fix:** Prepare and apply all mounts before calling `namespace.RunInNewNamespaceWithCgroup()`

### Port Forwarding Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Host (localhost:8080 or external_ip:8080)                   │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
            ┌──────────────────┐
            │ iptables DNAT    │ (PREROUTING/OUTPUT)
            │ :8080 -> IP:80   │
            └─────────┬────────┘
                      │
                      ▼
            ┌──────────────────┐
            │ minidocker0      │ (bridge)
            │ 172.18.0.1/24    │
            └─────────┬────────┘
                      │
                      ▼
            ┌──────────────────┐
            │ veth pair        │
            └─────────┬────────┘
                      │
                      ▼
            ┌──────────────────┐
            │ Container        │
            │ eth0: 172.18.0.X │
            │ Port 80          │
            └──────────────────┘
```

### Volume Architecture

```
Host Filesystem                    Container Filesystem
─────────────────                  ────────────────────

/var/lib/minidocker/volumes/       
├── myvolume/                      /data (inside container)
│   ├── _data_/          ────────> (bind mounted)
│   └── metadata.json              
                                   
/host/app/logs/          ────────> /var/log (bind mount)
```

### Testing Results

**Port Forwarding:**
- ✅ TCP port forwarding works (`-p 8080:80`)
- ✅ UDP port forwarding works (`-p 5353:53/udp`)
- ✅ Multiple ports per container (`-p 8080:80 -p 9090:9000`)
- ✅ Localhost access works (`curl http://127.0.0.1:8080`)
- ✅ External access works (from host IP)
- ✅ Port validation (1-65535 range)
- ✅ Port availability check before binding
- ✅ Automatic cleanup on container stop
- ✅ `port` command shows active mappings

**Volume Management:**
- ✅ Named volumes created and persisted
- ✅ Bind mounts from host paths
- ✅ Read-only mount flag works (`:ro`)
- ✅ Multiple volumes per container
- ✅ Automatic volume creation on first use
- ✅ Volume metadata tracking
- ✅ Volume listing and inspection
- ✅ Volume removal (when not in use)
- ✅ Cleanup on container exit

### Key Learnings

**Port Forwarding:**
- `route_localnet=1` is essential for localhost port forwarding
- Need separate iptables rules for localhost (OUTPUT) vs external (PREROUTING)
- MASQUERADE handles both SNAT and dynamic IP scenarios
- IPv6 requires separate ip6tables rules or explicit IPv4 usage
- tcpdump is invaluable for debugging network traffic flow
- Connection resets can be normal behavior (not always errors)

**Volume Management:**
- Bind mounts must be applied before container starts (before chroot)
- Volume vs bind mount distinction based on path format
- Read-only flag requires remount with `mount -o remount,ro`
- Cleanup must unmount in reverse order to avoid busy filesystem
- Custom flag types enable Docker-like CLI interface
- Volume data persists even when containers are removed

### Container Metadata Schema (Updated)

```json
{
  "id": "c1761986867777401860",
  "name": "c1761986867777401860",
  "image": "ubuntu",
  "command": ["python3", "-m", "http.server", "80"],
  "state": "running",
  "pid": 2192254,
  "exit_code": 0,
  "created": "2025-11-01T08:47:47Z",
  "started": "2025-11-01T08:47:47Z",
  "finished": "0001-01-01T00:00:00Z",
  "log_path": "/var/lib/minidocker/containers/c1761986867777401860.log",
  "ip_address": "172.18.0.59/24",
  "network_mode": "bridge",
  "mounts": [
    {
      "type": "volume",
      "source": "mydata",
      "destination": "/data",
      "read_only": false
    }
  ],
  "ports": [
    {
      "host_port": 8080,
      "container_port": 80,
      "protocol": "tcp"
    }
  ]
}
```

### Current Capabilities Summary

Containers now support:
- ✅ Full lifecycle management (create, run, stop, remove, exec)
- ✅ Resource limits (memory, CPU via cgroups)
- ✅ Network isolation with Internet access
- ✅ **Port forwarding from host to container**
- ✅ **Persistent data with volumes**
- ✅ **Bind mounts from host filesystem**
- ✅ Detached mode execution
- ✅ Log capture and viewing
- ✅ Multiple containers on same bridge
- ✅ Automatic cleanup and resource management

---

**Next Steps (Phase 3 - Advanced Features)**
- [ ] Container-to-container networking (service discovery)
- [ ] DNS resolution inside containers (`/etc/resolv.conf`)
- [ ] Custom networks (`--net=custom-bridge`)
- [ ] Network modes: `--net=none`, `--net=host`
- [ ] Image management (pull, build, push)
- [ ] Dockerfile support
- [ ] Multi-container orchestration
- [ ] Container health checks
- [ ] Resource usage statistics (`stats` command)
- [ ] Container restart policies
