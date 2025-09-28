package main

import (
	"fmt"
	"os"
	"github.com/jagjeet-singh-23/minidocker/pkg/namespace"
	"github.com/jagjeet-singh-23/minidocker/pkg/image"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: minidocker <command> [args...]")
		fmt.Println("Commands:")
		fmt.Println(" run <image> <command> - Run a container")
		fmt.Println(" images                - List available images")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "run":
		if len(os.Args) < 4 {
			fmt.Println("Usage: minidocker <image> <command>")
			os.Exit(1)
		}
		runContainer(os.Args[2], os.Args[3:])
	case "images":
		listImages()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}

func runContainer(imageName string, command []string) {
	fmt.Printf("Running container with image: %s", imageName)
	
	// Get the rootfs path
	rootfsPath, err := image.GetImageRootfs(imageName)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Run container with rootfs
	if err := namespace.RunInNewNamespace(command, rootfsPath); err != nil {
		fmt.Println("Error running container: %v\n", err)
		os.Exit(1)
	}
}

func listImages() {
	images, err := image.ListImages()
	if err != nil {
		fmt.Println("Error listing images: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Available images:")
	for _, img := range images {
		fmt.Println("  %s\n", img)
	}
}
