package main

import (
	"fmt"
	"os"
	"github.com/jagjeet-singh-23/minidocker/pkg/namespace"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: minidocker <command> [args...]")
		fmt.Println("Commands:")
		fmt.Println(" run <image> <command> - Run a container")
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
	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}

func runContainer(image string, command []string) {
	fmt.Printf("Running container with image: %s, command: %v\n", image, command)

	// For now, ignore image and just run command in namespace
	if err:= namespace.RunInNewNamespace(command); err != nil {
		fmt.Printf("Error running container: %v\n", err)
		os.Exit(1)
	}
}
