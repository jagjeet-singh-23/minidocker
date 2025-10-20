package main

import (
    "flag"
    "fmt"
    "os"
    "os/exec"
    "strconv"
    "syscall"
    "text/tabwriter"
    "time"
    "github.com/jagjeet-singh-23/minidocker/pkg/cgroup"
    "github.com/jagjeet-singh-23/minidocker/pkg/container"
    "github.com/jagjeet-singh-23/minidocker/pkg/image"
    "github.com/jagjeet-singh-23/minidocker/pkg/namespace"
    "github.com/jagjeet-singh-23/minidocker/pkg/network"
)

func main() {
    if len(os.Args) < 2 {
        fmt.Println("Usage: minidocker <command> [args...]")
        fmt.Println("Commands:")
        fmt.Println("  run [--memory=MB] [--cpu=CORES] [--net=MODE] [-d] <image> <command>    - Run a container")
        fmt.Println("  ps                                                                     - List containers")
        fmt.Println("  stop <container-id>                                                    - Stop a container")
        fmt.Println("  rm <container-id>                                                      - Remove a container")
        fmt.Println("  logs <container-id>                                                    - Show container logs")
        fmt.Println("  images                                                                 - List available images")
	fmt.Println("  exec <container-id> <command>				              - Execute command in running container")
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
    case "logs":
    	    showLogs()
    case "images":
        listImages()
    case "exec":
	    execContainer()
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

    // Get image rootfs path
    rootfsPath, err := image.GetImageRootfs(imageName)
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        os.Exit(1)
    }

    // Create container metadata
    containerID := container.GenerateContainerID()
    logPath := fmt.Sprintf("/var/lib/minidocker/containers/%s.log", containerID)

    containerInfo := &container.Container{
        ID: containerID,
        Name: containerID,
        Image: imageName,
        Command: command,
        State: container.StateCreated,
        Created: time.Now(),
        LogPath: logPath,
	NetworkMode: *networkMode,
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

    // Setup container network if bridge mode
    if enableNetwork {
	    containerIP, err := network.SetupContainerNetwork(containerID, pid)
	    if err != nil {
		    fmt.Printf("Warning: failed to setup network: %v\n", err)
	    } else {
		    containerInfo.IPAddress = containerIP
		    container.SaveContainer(containerInfo)
		    fmt.Printf("Container network configured with IP: %s\n", containerIP)
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

	    // Cleanup network
	    if enableNetwork {
		    network.CleanupContainerNetwork(containerID)
	    }
            
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

    if enableNetwork {
	    network.CleanupContainerNetwork(containerID)
    }
    
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
