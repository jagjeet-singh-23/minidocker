package main

import (
    "flag"
    "fmt"
    "os"
    "time"
    "github.com/jagjeet-singh-23/minidocker/pkg/namespace"
    "github.com/jagjeet-singh-23/minidocker/pkg/image"
    "github.com/jagjeet-singh-23/minidocker/pkg/cgroup"
)

func main() {
    if len(os.Args) < 2 {
        fmt.Println("Usage: minidocker <command> [args...]")
        fmt.Println("Commands:")
        fmt.Println("  run [--memory=MB] [--cpu=CORES] <image> <command>    - Run a container")
        fmt.Println("  images                                                - List available images")
        os.Exit(1)
    }

    command := os.Args[1]
    
    switch command {
    case "run":
        runContainer()
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
    
    runCmd.Parse(os.Args[2:])
    
    args := runCmd.Args()
    if len(args) < 2 {
        fmt.Println("Usage: minidocker run [--memory=MB] [--cpu=CORES] <image> <command>")
        os.Exit(1)
    }
    
    imageName := args[0]
    command := args[1:]
    
    fmt.Printf("Starting container with image: %s\n", imageName)
    if *memoryMB > 0 {
        fmt.Printf("Memory limit: %d MB\n", *memoryMB)
    }
    if *cpuCores > 0 {
        fmt.Printf("CPU limit: %.2f cores\n", *cpuCores)
    }
    
    // Get image rootfs path
    rootfsPath, err := image.GetImageRootfs(imageName)
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        os.Exit(1)
    }
    
    // Generate container ID
    containerID := fmt.Sprintf("c%d", time.Now().Unix())
    
    // Set up cgroups if limits specified
    if *memoryMB > 0 || *cpuCores > 0 {
        limits := cgroup.ContainerLimits{
            MemoryMB: *memoryMB,
            CPUQuota: *cpuCores,
        }
        
        if err := cgroup.CreateCgroupForContainer(containerID, limits); err != nil {
            fmt.Printf("Error creating cgroup: %v\n", err)
            os.Exit(1)
        }
        
        // Clean up cgroup on exit
        defer cgroup.RemoveCgroup(containerID)
    }
    
    // Run container with cgroup
    if err := namespace.RunInNewNamespaceWithCgroup(command, rootfsPath, containerID); err != nil {
        fmt.Printf("Error running container: %v\n", err)
        os.Exit(1)
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
