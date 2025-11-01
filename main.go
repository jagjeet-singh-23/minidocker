package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
    "syscall"
    "text/tabwriter"
    "time"
    "github.com/jagjeet-singh-23/minidocker/pkg/cgroup"
    "github.com/jagjeet-singh-23/minidocker/pkg/container"
    "github.com/jagjeet-singh-23/minidocker/pkg/image"
    "github.com/jagjeet-singh-23/minidocker/pkg/namespace"
    "github.com/jagjeet-singh-23/minidocker/pkg/network"
    "github.com/jagjeet-singh-23/minidocker/pkg/volume"
)

func main() {
    if len(os.Args) < 2 {
        fmt.Println("Usage: minidocker <command> [args...]")
        fmt.Println("Commands:")
        fmt.Println("  run [--memory=MB] [--cpu=CORES] [--net=MODE] [-d] [-v SOURCE:DEST[:ro]] <image> <command>")
        fmt.Println("  ps                                           - List containers")
        fmt.Println("  stop <container-id>                          - Stop a container")
        fmt.Println("  rm <container-id>                            - Remove a container")
        fmt.Println("  exec <container-id> <command>                - Execute in container")
        fmt.Println("  logs <container-id>                          - Show container logs")
        fmt.Println("  images                                       - List available images")
        fmt.Println("  volume create <name>                         - Create a volume")
        fmt.Println("  volume ls                                    - List volumes")
        fmt.Println("  volume rm <name>                             - Remove a volume")
        fmt.Println("  volume inspect <name>                        - Inspect a volume")
        os.Exit(1)
    }

    command := os.Args[1]
    
    switch command {
    case "run":
        runContainer()
    case "ps":
        listContainers()
    case "stop":
        stopContainer()
    case "rm":
        removeContainer()
    case "exec":
        execContainer()
    case "logs":
        showLogs()
    case "images":
        listImages()
    case "volume":
        handleVolumeCommand()
    case "port":
	showPorts()
    default:
        fmt.Printf("Unknown command: %s\n", command)
        os.Exit(1)
    }
}

func runContainer() {
    // Parse run command flags
    runCmd := flag.NewFlagSet("run", flag.ExitOnError)
    memoryMB := runCmd.Int("memory", 0, "Memory limit in MB")
    cpuCores := runCmd.Float64("cpu", 0, "CPU limit (e.g., 0.5 for half a core)")
    detach := runCmd.Bool("d", false, "Run container in background")
    networkMode := runCmd.String("net", "bridge", "Network mode (bridge or none)")

    var volumeSpecs arrayFlags
    var portSpecs arrayFlags
    runCmd.Var(&volumeSpecs, "v", "Volume mount (can be repeated): -v /host/path:/container/path[:ro] or -v volumename:/container/path[:ro]")
    runCmd.Var(&portSpecs, "p", "Port mapping (can be repeated): -p HOST_PORT:CONTAINER_PORT[/PROTOCOL]")
    
    runCmd.Parse(os.Args[2:])
    
    args := runCmd.Args()
    if len(args) < 2 {
        fmt.Println("Usage: minidocker run [--memory=MB] [--cpu=CORES] <image> <command>")
        os.Exit(1)
    }
    
    imageName := args[0]
    command := args[1:]

    // Validate network mode
    if *networkMode != "bridge" && *networkMode != "none" {
	    fmt.Printf("Invalid network mode: %s (use 'bridge' or 'none')", *networkMode)
	    os.Exit(1)
    }

    if len(portSpecs) > 0 && *networkMode != "bridge" {
	    fmt.Println("Error: Port mapping requires bridge networking (--net=bridge)")
	    os.Exit(1)
    }

    // Get image rootfs path
    rootfsPath, err := image.GetImageRootfs(imageName)
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        os.Exit(1)
    }

    // Parse volume spefications
    var mounts []volume.Mount
    for _, volSpec := range volumeSpecs {
	    mount, err := parseVolumeSpec(volSpec)
	    if err != nil {
		    fmt.Printf("Error parsing volume spec '%s': %v\n", volSpec, err)
		    os.Exit(1)
	    }
	    mounts = append(mounts, *mount)
    }

    // Parse port specifications
    var ports[]container.PortMapping
    for _, portSpec := range portSpecs {
	    portMapping, err := parsePortSpec(portSpec)
	    if err != nil {
		    fmt.Println("Error parsing port spec '%s': %v\n", portSpec, err)
		    os.Exit(1)
	    }
	    ports = append(ports, *portMapping)
    }

    // Create container metadata
    containerID := container.GenerateContainerID()
    logPath := fmt.Sprintf("/var/lib/minidocker/containers/%s.log", containerID)

    containerInfo := &container.Container{
        ID: 	     containerID,
        Name: 	     containerID,
        Image: 	     imageName,
        Command:     command,
        State:       container.StateCreated,
        Created:     time.Now(),
        LogPath:     logPath,
	NetworkMode: *networkMode,
	Mounts:      mounts,
	Ports:       ports,
    }

    if err := container.SaveContainer(containerInfo); err != nil {
        fmt.Printf("Error saving container: %v\n", err)
        os.Exit(1)
    }

    fmt.Printf("Container %s created\n", containerID)
    
    if *memoryMB > 0 || *cpuCores > 0 {
        limits := cgroup.ContainerLimits{
            MemoryMB: *memoryMB,
            CPUQuota: *cpuCores,
        }

        if err := cgroup.CreateCgroupForContainer(containerID, limits); err != nil {
            fmt.Printf("Error creating cgroup: %v\n", err)
            os.Exit(1)
        }
        
        // Don't defer if detached - let goroutine handle cleanup
        if !*detach {
            defer cgroup.RemoveCgroup(containerID)
        }
    }

    // Setup bridge network if needed
    enableNetwork := *networkMode == "bridge"
    if enableNetwork {
	    if err := network.SetupBridge(); err != nil {
		    fmt.Printf("Error setting up bridge: %v\n", err)
		    os.Exit(1)
	    }
    }

    // Prepare and apply volume mounts BEFORE starting container
    for i, mount := range mounts {
	    sourcePath, err := volume.PrepareMount(&mount)
	    if err != nil {
		    fmt.Printf("Error preparing mount: %v\n", err)
		    os.Exit(1)
	    }

	    // Apply mount to rootfs
	    if err := applyMountToRootfs(rootfsPath, sourcePath, mount.Destination, mount.ReadOnly); err != nil {
		    fmt.Printf("Error applying mount: %v\n", err)
		    os.Exit(1)
	    }

	    fmt.Printf("Mounted %s to %s\n", mount.Source, mount.Destination)

	    // Update mount with actial source path
	    mounts[i].Source = sourcePath
    }

    // Update state to running
    containerInfo.State = container.StateRunning
    containerInfo.Started = time.Now()
    container.SaveContainer(containerInfo)

    // Run container and capture PID
    pid, err := namespace.RunInNewNamespaceWithCgroup(command, rootfsPath, containerID, enableNetwork)
    
    if err != nil {
        fmt.Printf("Error starting container: %v\n", err)
        os.Exit(1)
    }
    
    // Save PID immediately
    containerInfo.PID = pid
    container.SaveContainer(containerInfo)

    var containerIP string
    // Setup container network if bridge mode
    if enableNetwork {
	    containerIP, err := network.SetupContainerNetwork(containerID, pid)
	    if err != nil {
		    fmt.Printf("Warning: failed to setup network: %v\n", err)
	    } else {
		    containerInfo.IPAddress = containerIP
		    container.SaveContainer(containerInfo)
		    fmt.Printf("Container network configured with IP: %s\n", containerIP)

		    if len(ports) > 0 && containerIP != "" {
			    ip := strings.Split(containerIP, "/")[0]

			    for _, portMapping := range ports {
				    err := network.SetupPortForwarding(
					    portMapping.HostPort,
					    portMapping.ContainerPort,
					    ip,
					    portMapping.Protocol,
				    )
				    if err != nil {
					    fmt.Printf("WARNING: failed to setup port forwarding %d:%d/%s: %v\n",
						portMapping.HostPort, portMapping.ContainerPort, portMapping.Protocol, err)
				    } else {
					    fmt.Printf("Port mapping: %d -> %s:%d/%s\n",
					    	portMapping.HostPort, ip, portMapping.ContainerPort, portMapping.Protocol)
				    }
			    }
		    }
	    }
    }
    
    if *detach {
        // Detach mode - run in goroutine
        fmt.Printf("Container %s started in background with PID %d\n", containerID[:12], pid)
        
        // Start goroutine to monitor container
        go func() {
            // Wait for container to finish
            process, _ := os.FindProcess(pid)
            processState, waitErr := process.Wait()
            
            exitCode := 0
            if waitErr != nil {
                exitCode = 1
            } else if processState != nil && !processState.Success() {
                exitCode = processState.ExitCode()
            }

            // Cleanup port forwarding
            if enableNetwork && containerIP != "" && len(ports) > 0 {
                ip := strings.Split(containerIP, "/")[0]
                for _, portMapping := range ports {
                    network.RemovePortForwarding(
                        portMapping.HostPort,
                        portMapping.ContainerPort,
                        ip,
                        portMapping.Protocol,
                    )
                }
            }

	    // Cleanup network
            if enableNetwork {
                network.CleanupContainerNetwork(containerID)
            }

	    // Clean up mounts
	    cleanupMounts(rootfsPath, mounts)
            
            // Update final state
            containerInfo.State = container.StateExited
            containerInfo.Finished = time.Now()
            containerInfo.ExitCode = exitCode
            containerInfo.PID = 0
	    containerInfo.IPAddress = ""
            container.SaveContainer(containerInfo)
            
            // Cleanup cgroup
            if *memoryMB > 0 || *cpuCores > 0 {
                cgroup.RemoveCgroup(containerID)
            }
        }()
        
        // Return immediately, don't wait
        return
    }
    
    // Foreground mode - wait for completion
    fmt.Printf("Container running with PID %d\n", pid)
    process, _ := os.FindProcess(pid)
    processState, err := process.Wait()
    
    exitCode := 0
    if err != nil {
        fmt.Printf("Container exited with error: %v\n", err)
        exitCode = 1
    } else if processState != nil && !processState.Success() {
        exitCode = processState.ExitCode()
    }

    // Cleanup port forwarding
    if enableNetwork && containerIP != "" && len(ports) > 0 {
	    ip := strings.Split(containerIP, "/")[0]
	    for _, portMapping := range ports {
		    network.RemovePortForwarding(
			    portMapping.HostPort,
			    portMapping.ContainerPort,
			    ip,
			    portMapping.Protocol,
		    )
	    }
    }

    // Cleanup network
    if enableNetwork {
	    network.CleanupContainerNetwork(containerID)
    }

    cleanupMounts(rootfsPath, mounts)
    
    // Update final state
    containerInfo.State = container.StateExited
    containerInfo.Finished = time.Now()
    containerInfo.ExitCode = exitCode
    containerInfo.PID = 0
    containerInfo.IPAddress = ""
    container.SaveContainer(containerInfo)
}

func listContainers() {
	containers, err := container.ListContainers()
	if err != nil {
		fmt.Println("Error listing containers: %v\n", err)
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CONTAINER ID\tIMAGE\tCOMMAND\tSTATE\tCREATED")

	for _, c := range containers {
		created := c.Created.Format("2006-01-02 15:04:05")
		commandStr := ""
		if len(c.Command) > 0 {
			commandStr = c.Command[0]
			if len(c.Command) > 1 {
				commandStr += "..."
			}
		}
	        fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", 
            		c.ID[:12], c.Image, commandStr, c.State, created)
	}
	w.Flush()
}

func stopContainer() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: minidocker stop <container-id>")
		os.Exit(1)
	}

	containerID := os.Args[2]
	containerInfo, err := container.FindContainerByPrefix(containerID)

	if err != nil {
		fmt.Printf("Container %s not found \n", containerID)
		os.Exit(1)
	}

	if containerInfo.State != container.StateRunning {
		fmt.Printf("Container %s is not running (state: %s)\n", containerInfo.ID[:12], containerInfo.State)
		return
	}

	if containerInfo.PID > 0 {
		if err := syscall.Kill(containerInfo.PID, syscall.SIGTERM); err != nil {
			fmt.Printf("Error stopping container: %v\n", err)
		} else {
			containerInfo.State = container.StateStopped
			containerInfo.Finished = time.Now()
			container.SaveContainer(containerInfo)
			fmt.Printf("Container %s stopped\n", containerID)
		}
	}
}

func removeContainer() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: minidocker rm <container-id>")
		os.Exit(1)
	}

	containerID := os.Args[2]
	containerInfo, err := container.FindContainerByPrefix(containerID)

	if err != nil {
		fmt.Printf("Error removing container: %v\n", err)
		os.Exit(1)
	}

	if containerInfo.State == container.StateRunning {
		fmt.Printf("Cannot remove running container %s. Stop it first.\n", containerID)
		os.Exit(1)
	}

	if err := container.RemoveContainer(containerID); err != nil {
		fmt.Printf("Error removing container: %v\n", err)
		os.Exit(1)
	}

	// Clean up resources
	cgroup.RemoveCgroup(containerID)

	fmt.Printf("Container %s removed\n", containerID)
}

func showLogs() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: minidocker logs <container-id>")
		os.Exit(1)
	}

	containerID := os.Args[2]
	containerInfo, err := container.LoadContainer(containerID)

	if err != nil {
		fmt.Printf("Container %s not found", containerID)
		os.Exit(1)
	}

	if containerInfo.LogPath == "" {
		fmt.Println("No logs available")
		os.Exit(1)
	}

	if data, err := os.ReadFile(containerInfo.LogPath); err != nil {
		fmt.Println("No logs available")
		os.Exit(1)
	} else {
		fmt.Print(string(data))
	}
}

func listImages() {
    images, err := image.ListImages()
    if err != nil {
        fmt.Printf("Error listing images: %v\n", err)
        os.Exit(1)
    }

    fmt.Println("Available images:")
    for _, img := range images {
        fmt.Printf("  %s\n", img)
    }
}

func execContainer() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: minidocker exec <container-id> <command>")
		os.Exit(1)
	}

	containerID := os.Args[2]
	command := os.Args[3:]

	// Find the container by prefix
	containerInfo, err := container.FindContainerByPrefix(containerID)
	if err != nil {
		fmt.Printf(
			"Container %s is not running (state: %s)\n", 
			containerInfo.ID[:12], 
			containerInfo.State,
		)
		os.Exit(1)
	}

	if containerInfo.State != container.StateRunning {
		fmt.Printf("Container %s is not running (state: %s)\n", containerInfo.ID[:12], containerInfo.State)
		os.Exit(1)
	}

	// Use the nsenter command to enter the container namespace
	nsenterArgs := []string{
		"nsenter",
		"--target",
		strconv.Itoa(containerInfo.PID),
		"--pid",
		"--uts",
		"--mount",
	}
	nsenterArgs = append(nsenterArgs, command...)

	cmd := exec.Command(nsenterArgs[0], nsenterArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Error executing command:  %v\n", err)
		os.Exit(1)
	}
}

func handleVolumeCommand() {
    if len(os.Args) < 3 {
        fmt.Println("Usage: minidocker volume <subcommand>")
        fmt.Println("Subcommands:")
        fmt.Println("  create <name>    - Create a volume")
        fmt.Println("  ls               - List volumes")
        fmt.Println("  rm <name>        - Remove a volume")
        fmt.Println("  inspect <name>   - Inspect a volume")
        os.Exit(1)
    }
    
    subcommand := os.Args[2]
    
    switch subcommand {
    case "create":
        volumeCreate()
    case "ls":
        volumeList()
    case "rm":
        volumeRemove()
    case "inspect":
        volumeInspect()
    default:
        fmt.Printf("Unknown volume subcommand: %s\n", subcommand)
        os.Exit(1)
    }
}

func volumeCreate() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: minidocker volume create <name>")
		os.Exit(1)
	}

	volumeName := os.Args[3]

	vol, err := volume.CreateVolume(volumeName)
	if err != nil {
		fmt.Printf("Error creating volume: %v", err)
		os.Exit(1)
	}

	fmt.Printf("Volume %s created\n", vol.Name)
}

func volumeList() {
    volumes, err := volume.ListVolumes()
    if err != nil {
        fmt.Printf("Error listing volumes: %v\n", err)
        os.Exit(1)
    }

    if len(volumes) == 0 {
        fmt.Println("No volumes found")
        return
    }

    w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
    fmt.Fprintln(w, "VOLUME NAME\tDRIVER\tCREATED")

    for _, vol := range volumes {
        created := vol.Created.Format("2006-01-02 15:04:05")
        fmt.Fprintf(w, "%s\t%s\t%s\n", vol.Name, vol.Driver, created)
    }
    w.Flush()
}

func volumeRemove() {
    if len(os.Args) < 4 {
        fmt.Println("Usage: minidocker volume rm <name>")
        os.Exit(1)
    }

    volumeName := os.Args[3]

    if err := volume.RemoveVolume(volumeName); err != nil {
        fmt.Printf("Error removing volume: %v\n", err)
        os.Exit(1)
    }

    fmt.Printf("Volume %s removed\n", volumeName)
}

func volumeInspect() {
    if len(os.Args) < 4 {
        fmt.Println("Usage: minidocker volume rm <name>")
        os.Exit(1)
    }

    volumeName := os.Args[3]

    vol, err := volume.GetVolume(volumeName)
    if err != nil {
	    fmt.Printf("Error: %v\n", err)
	    os.Exit(1)
    }

    data, _ := json.MarshalIndent(vol, "", "  ")
    fmt.Println(string(data))

}

// Custom flag type for multiple -v flags
type arrayFlags []string

func (i *arrayFlags) String() string {
    return strings.Join(*i, ", ")
}

func (i *arrayFlags) Set(value string) error {
    *i = append(*i, value)
    return nil
}

// parseVolumeSpec parses volume specification: SOURCE:DEST[:ro]
func parseVolumeSpec(spec string) (*volume.Mount, error) {
    parts := strings.Split(spec, ":")

    if len(parts) < 2 {
        return nil, fmt.Errorf("invalid volume spec format (use SOURCE:DEST or SOURCE:DEST:ro)")
    }

    mount := &volume.Mount{
        Source:      parts[0],
        Destination: parts[1],
        ReadOnly:    false,
    }

    // Determine if it's a bind mount or volume
    if filepath.IsAbs(parts[0]) {
        mount.Type = "bind"
    } else {
        mount.Type = "volume"
    }

    // Check for read-only flag
    if len(parts) >= 3 && parts[2] == "ro" {
        mount.ReadOnly = true
    }

    return mount, nil
}

// applyMountToRootfs performs bind mount into container rootfs
func applyMountToRootfs(rootfsPath, sourcePath, destPath string, readOnly bool) error {
    targetPath := filepath.Join(rootfsPath, destPath)

    // Create mount point directory
    if err := os.MkdirAll(targetPath, 0755); err != nil {
        return fmt.Errorf("failed to create mount point: %v", err)
    }

    // Perform bind mount
    mountCmd := []string{"mount", "--bind", sourcePath, targetPath}
    cmd := exec.Command(mountCmd[0], mountCmd[1:]...)

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to bind mount: %v", err)
    }

    // Apply read-only remount if needed
    if readOnly {
        remountCmd := exec.Command("mount", "-o", "remount,ro,bind", targetPath)
        if err := remountCmd.Run(); err != nil {
            return fmt.Errorf("failed to remount as read-only: %v", err)
        }
    }

    return nil
}

// cleanupMounts unmounts all volume mounts
func cleanupMounts(rootfsPath string, mounts []volume.Mount) {
    for _, mount := range mounts {
        targetPath := filepath.Join(rootfsPath, mount.Destination)
        exec.Command("umount", targetPath).Run()
    }
}

// parsePortSpec parses port specification: HOST_PORT:CONTAINER_PORT[/PROTOCOL]
func parsePortSpec(spec string) (*container.PortMapping, error) {
    // Split by slash to get protocol
    parts := strings.Split(spec, "/")
    portPart := parts[0]
    protocol := "tcp" // default

    if len(parts) == 2 {
        protocol = strings.ToLower(parts[1])
        if protocol != "tcp" && protocol != "udp" {
            return nil, fmt.Errorf("invalid protocol: %s (use tcp or udp)", protocol)
        }
    }

    // Split port part by colon
    portParts := strings.Split(portPart, ":")
    if len(portParts) != 2 {
        return nil, fmt.Errorf("invalid port format (use HOST_PORT:CONTAINER_PORT)")
    }

    hostPort, err := strconv.Atoi(portParts[0])
    if err != nil {
        return nil, fmt.Errorf("invalid host port: %s", portParts[0])
    }

    containerPort, err := strconv.Atoi(portParts[1])
    if err != nil {
        return nil, fmt.Errorf("invalid container port: %s", portParts[1])
    }

    if err := network.ValidatePort(hostPort); err != nil {
        return nil, fmt.Errorf("host port: %v", err)
    }

    if err := network.ValidatePort(containerPort); err != nil {
        return nil, fmt.Errorf("container port: %v", err)
    }

    // Check if host port is available
    if !network.CheckPortAvailable(hostPort) {
        return nil, fmt.Errorf("host port %d is already in use", hostPort)
    }

    return &container.PortMapping{
        HostPort:      hostPort,
        ContainerPort: containerPort,
        Protocol:      protocol,
    }, nil
}

func showPorts() {
    if len(os.Args) < 3 {
        fmt.Println("Usage: minidocker port <container-id>")
        os.Exit(1)
    }
    
    containerID := os.Args[2]
    containerInfo, err := container.FindContainerByPrefix(containerID)
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        os.Exit(1)
    }
    
    if len(containerInfo.Ports) == 0 {
        fmt.Println("No port mappings")
        return
    }
    
    fmt.Println("PORT MAPPINGS:")
    for _, port := range containerInfo.Ports {
        fmt.Printf("%d/%s -> %s:%d\n",
            port.HostPort, port.Protocol,
            strings.Split(containerInfo.IPAddress, "/")[0], port.ContainerPort)
    }
}
