package network

import (
    crand "crypto/rand"
    "encoding/hex"
    "fmt"
    mrand "math/rand"
    "net"
    "os"
    "os/exec"
    "regexp"
    "strings"
    "time"
)

const (
    BridgeName = "minidocker0"
    SubnetCIDR = "172.18.0.0/24"
    BridgeIP   = "172.18.0.1/24"
)

// SetupBridge creates the minidocker bridge if it doesn't exist
func SetupBridge() error {
    // Check if bridge exists
    cmd := exec.Command("ip", "link", "show", BridgeName)
    bridgeExists := cmd.Run() == nil

    if !bridgeExists {
        // Create bridge
        if err := exec.Command("ip", "link", "add", BridgeName, "type", "bridge").Run(); err != nil {
            return fmt.Errorf("failed to create bridge: %v", err)
        }

        // Set bridge IP
        if err := exec.Command("ip", "addr", "add", BridgeIP, "dev", BridgeName).Run(); err != nil {
            return fmt.Errorf("failed to set bridge IP: %v", err)
        }
    }

    // Always ensure bridge is up
    if err := exec.Command("ip", "link", "set", BridgeName, "up").Run(); err != nil {
        return fmt.Errorf("failed to bring bridge up: %v", err)
    }

    // Enable IP forwarding
    if err := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run(); err != nil {
        return fmt.Errorf("failed to enable IP forwarding: %v", err)
    }

    // Setup NAT (check if rule already exists)
    if err := setupNAT(); err != nil {
        // Ignore if rule already exists
        fmt.Printf("NAT setup note: %v\n", err)
    }

    exec.Command("iptables", "-A", "FORWARD", "-i", BridgeName, "-j", "ACCEPT").Run()
    exec.Command("iptables", "-A", "FORWARD", "-o", BridgeName, "-j", "ACCEPT").Run()

    return nil
}

func detectExternalInterface() (string, error) {
    out, err := exec.Command("sh", "-c",
        "ip route get 8.8.8.8 | awk '{for(i=1;i<=NF;i++){if($i==\"dev\"){print $(i+1); exit}}}'").Output()
    if err != nil {
        return "", err
    }
    return strings.TrimSpace(string(out)), nil
}

// setupNAT configures iptables for container internet access
func setupNAT() error {
    extIf, err := detectExternalInterface()
    if err != nil {
        return fmt.Errorf("cannot detect external interface: %v", err)
    }

    cmd := exec.Command("iptables", "-t", "nat", "-C", "POSTROUTING",
        "-s", SubnetCIDR, "-o", extIf, "-j", "MASQUERADE")
    if cmd.Run() == nil {
        return nil
    }

    return exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING",
        "-s", SubnetCIDR, "-o", extIf, "-j", "MASQUERADE").Run()
}

// SetupContainerNetwork creates veth pair and connects to bridge
func SetupContainerNetwork(containerID string, pid int) (string, error) {
    // Generate interface names
    randomBytes := make([]byte, 4)
    if _, err := crand.Read(randomBytes); err != nil {
        return "", fmt.Errorf("failed to read random bytes: %v", err)
    }
    suffix := hex.EncodeToString(randomBytes)[:6]

    vethHost := fmt.Sprintf("veth%s", suffix)
    vethContainer := fmt.Sprintf("vethc%s", suffix)

    // Create veth pair
    cmd := exec.Command("ip", "link", "add", vethHost, "type", "veth", "peer", "name", vethContainer)
    if err := cmd.Run(); err != nil {
        return "", fmt.Errorf("failed to create veth pair: %v", err)
    }

    // Attach host end to bridge
    if err := exec.Command("ip", "link", "set", vethHost, "master", BridgeName).Run(); err != nil {
        return "", fmt.Errorf("failed to attach veth to bridge: %v", err)
    }

    // Bring up host veth
    if err := exec.Command("ip", "link", "set", vethHost, "up").Run(); err != nil {
        return "", fmt.Errorf("failed to bring up host veth: %v", err)
    }

    // Move container end into container's network namespace
    nsPath := fmt.Sprintf("/proc/%d/ns/net", pid)
    if err := exec.Command("ip", "link", "set", vethContainer, "netns", nsPath).Run(); err != nil {
        return "", fmt.Errorf("failed to move veth to container: %v", err)
    }

    // Allocate IP for container
    containerIP, err := allocateIP(containerID)
    if err != nil {
        return "", err
    }

    // Configure container network from inside namespace
    if err := configureContainerNetNS(pid, vethContainer, containerIP); err != nil {
        return "", err
    }

    return containerIP, nil
}

// allocateIP assigns an IP address to the container
func allocateIP(containerID string) (string, error) {
    // Seed math/rand for pseudo-random IP allocation
    mrand.Seed(time.Now().UnixNano())

    // Use safe ranges: 1..254 for both octets, avoid .1 (bridge) and .0/.255
    hostPart := mrand.Intn(253) + 2
    return fmt.Sprintf("172.18.0.%d/24", hostPart), nil
}

// configureContainerNetNS sets up networking inside container namespace
func configureContainerNetNS(pid int, vethName, ipAddr string) error {
    netnsName := fmt.Sprintf("minidocker-%d", pid)
    netnsPath := fmt.Sprintf("/proc/%d/ns/net", pid)

    // Create netns directory if not exists
    exec.Command("mkdir", "-p", "/var/run/netns").Run()

    // Create syslink for ip netns command
    exec.Command("ln", "-sf", netnsPath, fmt.Sprintf("/var/run/netns/%s", netnsName)).Run()

    nsenterCmd := func(args ...string) error {
        fullArgs := []string{"ip", "netns", "exec", netnsName}
        fullArgs = append(fullArgs, args...)

        cmd := exec.Command(fullArgs[0], fullArgs[1:]...)
        output, err := cmd.CombinedOutput()
        if err != nil {
            return fmt.Errorf("%v: %s", err, string(output))
        }
        return nil
    }

    // Rename veth to eth0 inside container
    if err := nsenterCmd("ip", "link", "set", vethName, "name", "eth0"); err != nil {
        return fmt.Errorf("failed to rename veth: %v", err)
    }

    // Set IP address
    if err := nsenterCmd("ip", "addr", "add", ipAddr, "dev", "eth0"); err != nil {
        return fmt.Errorf("failed to set IP: %v", err)
    }

    // Bring up eth0
    if err := nsenterCmd("ip", "link", "set", "eth0", "up"); err != nil {
        return fmt.Errorf("failed to bring up eth0: %v", err)
    }

    // Bring up loopback
    if err := nsenterCmd("ip", "link", "set", "lo", "up"); err != nil {
        return fmt.Errorf("failed to bring up loopback: %v", err)
    }

    // Set default route via bridge
    err := nsenterCmd("ip", "route", "add", "default", "via", "172.18.0.1", "dev", "eth0")
    if err != nil && !strings.Contains(err.Error(), "File exists") {
        return fmt.Errorf("failed to set default route: %v", err)
    }

    // --- validation: ensure ipAddr is CIDR and gateway is inside same subnet ---
    _, ipNet, err := net.ParseCIDR(ipAddr)
    if err != nil || ipNet == nil {
	    return fmt.Errorf("invalid ipAddr %q: must be CIDR (e.g. 172.18.0.10/24)", ipAddr)
    }

    gw := net.ParseIP("172.18.0.1")
    if gw == nil {
	    return fmt.Errorf("invalid gateway IP")
    }
    if !ipNet.Contains(gw) {
	    return fmt.Errorf("gateway %s is not inside assigned network %s for container (ip %s)", gw.String(), ipNet.String(), ipAddr)
    }

    // --- validation: ensure target PID is in a different netns (not host) ---
    hostNS, err := os.Readlink("/proc/self/ns/net")
    if err != nil {
	    return fmt.Errorf("failed to read host netns: %v", err)
    }
    procNSPath := fmt.Sprintf("/proc/%d/ns/net", pid)
    procNS, err := os.Readlink(procNSPath)
    if err != nil {
	    return fmt.Errorf("failed to read netns for pid %d: %v", pid, err)
    }
    if hostNS == procNS {
	    return fmt.Errorf("pid %d appears to be in the host network namespace (%s); ensure process is created with CLONE_NEWNET", pid, procNS)
    }


    // Cleanup symlink
    defer exec.Command("rm", "-f", fmt.Sprintf("/var/run/netns/%s", netnsName)).Run()

    return nil
}

// CleanupContainerNetwork removes veth interfaces
func CleanupContainerNetwork(containerID string) error {
    vethHost := fmt.Sprintf("veth%s", containerID[:8])

    exec.Command("ip", "link", "delete", vethHost).Run()
    exec.Command("ip", "link", "delete", fmt.Sprintf("veth%s", containerID[:8])).Run()

    return nil
}

// GetContainerIP returns the IP address of a container
func GetContainerIP(containerID string, pid int) (string, error) {
    cmd := exec.Command("nsenter", "-t", fmt.Sprintf("%d", pid), "-n",
        "ip", "-4", "addr", "show", "eth0")

    output, err := cmd.Output()
    if err != nil {
        return "", err
    }

    return parseIPFromOutput(string(output)), nil
}

func parseIPFromOutput(output string) string {
    re := regexp.MustCompile(`inet\s+(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)
    matches := re.FindStringSubmatch(output)

    if len(matches) >= 2 {
        return matches[1]
    }

    return ""
}
