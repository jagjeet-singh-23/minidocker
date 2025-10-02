package main

import (
    "flag"
    "fmt"
    "os"
    "syscall"
    "text/tabwriter"
    "time"
    "github.com/jagjeet-singh-23/minidocker/pkg/cgroup"
    "github.com/jagjeet-singh-23/minidocker/pkg/container"
    "github.com/jagjeet-singh-23/minidocker/pkg/image"
    "github.com/jagjeet-singh-23/minidocker/pkg/namespace"
)

func main() {
    if len(os.Args) < 2 {
        fmt.Println("Usage: minidocker <command> [args...]")
        fmt.Println("Commands:")
        fmt.Println("  run [--memory=MB] [--cpu=CORES] [-d] <image> <command>    - Run a container")
        fmt.Println("  ps                                                        - List containers")
        fmt.Println("  stop <container-id>                                       - Stop a container")
        fmt.Println("  rm <container-id>                                         - Remove a container")
        fmt.Println("  logs <container-id>                                       - Show container logs")
        fmt.Println("  images                                                    - List available images")
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
    
    runCmd.Parse(os.Args[2:])
    
    args := runCmd.Args()
    if len(args) < 2 {
        fmt.Println("Usage: minidocker run [--memory=MB] [--cpu=CORES] <image> <command>")
        os.Exit(1)
    }
    
    imageName := args[0]
    command := args[1:]

    // Get image rootfs path
    rootfsPath, err := image.GetImageRootfs(imageName)
    if err != nil {
	    fmt.Println("Error: %v\n", err)
	    os.Exit(1)
    }

    // Create container metadata
    containerID := container.GenerateContainerID()
    logPath := fmt.Sprintf("/var/lib/minidocker/containers/%s.log", containerID)

    containerInfo := &container.Container{
	    ID: containerID,
	    Name: containerID,  // todo: update container name
	    Image: imageName,
	    Command: command,
	    State: container.StateCreated,
	    Created: time.Now(),
	    LogPath: logPath,
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

	    defer cgroup.RemoveCgroup(containerID)
    }

    // Update state to running
    containerInfo.State = container.StateRunning
    containerInfo.Started = time.Now()
    container.SaveContainer(containerInfo)

    if *detach {
	    fmt.Printf("Starting contianer %s in background\n", containerID)
	    // todo: handle background execution
    }

    // Run container
    exitCode := 0
    if err := namespace.RunInNewNamespaceWithCgroup(command, rootfsPath, containerID); err != nil {
	    fmt.Printf("Container exited with error: %v\n", err)
	    exitCode = 1
    }

    // Update final state
    containerInfo.State = container.StateExited
    containerInfo.Finished = time.Now()
    containerInfo.ExitCode = exitCode
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

