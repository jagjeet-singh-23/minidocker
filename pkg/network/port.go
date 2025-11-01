package network

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
)

// SetupPortForwarding configures iptables rules for port forwarding
func SetupPortForwarding(hostPort, containerPort int, containerIP, protocol string) error {
	if protocol != "tcp" && protocol != "udp" {
		return fmt.Errorf("invalid protocol: %s (use 'tcp' or 'udp')", protocol)
	}

	// Add DNAT rule to forward traffic from external interfaces to container
	dnatRule := []string{
		"iptables", "-t", "nat", "-A", "PREROUTING",
		"-p", protocol,
		"--dport", strconv.Itoa(hostPort),
		"-j", "DNAT",
		"--to-destination", fmt.Sprintf("%s:%d", containerIP, containerPort),
	}

	if err := exec.Command(dnatRule[0], dnatRule[1:]...).Run(); err != nil {
		return fmt.Errorf("failed to add DNAT rule: %v", err)
	}

	// Add DNAT rule for localhost traffic (OUTPUT chain)
	outputDnatRule := []string{
		"iptables", "-t", "nat", "-A", "OUTPUT",
		"-p", protocol,
		"-d", "127.0.0.1",
		"--dport", strconv.Itoa(hostPort),
		"-j", "DNAT",
		"--to-destination", fmt.Sprintf("%s:%d", containerIP, containerPort),
	}

	if err := exec.Command(outputDnatRule[0], outputDnatRule[1:]...).Run(); err != nil {
		return fmt.Errorf("failed to add OUTPUT DNAT rule: %v", err)
	}

	// Add SNAT rule for locally originated traffic to container
	localhostSnatRule := []string{
		"iptables", "-t", "nat", "-A", "POSTROUTING",
		"-p", protocol,
		"-s", "127.0.0.1",
		"-d", containerIP,
		"--dport", strconv.Itoa(containerPort),
		"-j", "MASQUERADE",
	}

	if err := exec.Command(localhostSnatRule[0], localhostSnatRule[1:]...).Run(); err != nil {
		fmt.Printf("Warning: failed to add localhost SNAT rule: %v\n", err)
	}
	
	// Add rule to allow forwarded traffic
	forwardRule := []string{
		"iptables", "-A", "FORWARD",
		"-p", protocol,
		"-d", containerIP,
		"--dport", strconv.Itoa(containerPort),
		"-j", "ACCEPT",
	}

	if err := exec.Command(forwardRule[0], forwardRule[1:]...).Run(); err != nil {
		return fmt.Errorf("failed to add FORWARD rule: %v", err)
	}

	// Add reverse FORWARD rule for established connections
	forwardReverseRule := []string{
		"iptables", "-A", "FORWARD",
		"-p", protocol,
		"-s", containerIP,
		"--sport", strconv.Itoa(containerPort),
		"-j", "ACCEPT",
	}

	if err := exec.Command(forwardReverseRule[0], forwardReverseRule[1:]...).Run(); err != nil {
		fmt.Printf("Warning: failed to add reverse FORWARD rule: %v\n", err)
	}

	// Add MASQUERADE for return traffic from container
	masqRule := []string{
		"iptables", "-t", "nat", "-A", "POSTROUTING",
		"-p", protocol,
		"-s", containerIP,
		"--sport", strconv.Itoa(containerPort),
		"-j", "MASQUERADE",
	}

	if err := exec.Command(masqRule[0], masqRule[1:]...).Run(); err != nil {
		return fmt.Errorf("failed to add MASQUERADE rule: %v", err)
	}

	fmt.Printf("Port forwarding rules added successfully for %d -> %s:%d/%s\n", 
		hostPort, containerIP, containerPort, protocol)

	return nil
}

// RemovePortForwarding removes iptables rules for port forwarding
func RemovePortForwarding(hostPort, containerPort int, containerIP, protocol string) error {
	// Remove DNAT rule
	dnatRule := []string{
		"iptables", "-t", "nat", "-D", "PREROUTING",
		"-p", protocol,
		"--dport", strconv.Itoa(hostPort),
		"-j", "DNAT",
		"--to-destination", fmt.Sprintf("%s:%d", containerIP, containerPort),
	}
	exec.Command(dnatRule[0], dnatRule[1:]...).Run()

	// Remove OUTPUT DNAT rule
	outputDnatRule := []string{
		"iptables", "-t", "nat", "-D", "OUTPUT",
		"-p", protocol,
		"-d", "127.0.0.1",
		"--dport", strconv.Itoa(hostPort),
		"-j", "DNAT",
		"--to-destination", fmt.Sprintf("%s:%d", containerIP, containerPort),
	}
	exec.Command(outputDnatRule[0], outputDnatRule[1:]...).Run()

	// Remove localhost SNAT rule
	localhostSnatRule := []string{
		"iptables", "-t", "nat", "-D", "POSTROUTING",
		"-p", protocol,
		"-s", "127.0.0.1",
		"-d", containerIP,
		"--dport", strconv.Itoa(containerPort),
		"-j", "MASQUERADE",
	}
	exec.Command(localhostSnatRule[0], localhostSnatRule[1:]...).Run()

	// Remove FORWARD rule
	forwardRule := []string{
		"iptables", "-D", "FORWARD",
		"-p", protocol,
		"-d", containerIP,
		"--dport", strconv.Itoa(containerPort),
		"-j", "ACCEPT",
	}
	exec.Command(forwardRule[0], forwardRule[1:]...).Run()

	// Remove reverse FORWARD rule
	forwardReverseRule := []string{
		"iptables", "-D", "FORWARD",
		"-p", protocol,
		"-s", containerIP,
		"--sport", strconv.Itoa(containerPort),
		"-j", "ACCEPT",
	}
	exec.Command(forwardReverseRule[0], forwardReverseRule[1:]...).Run()

	// Remove MASQUERADE rule
	masqRule := []string{
		"iptables", "-t", "nat", "-D", "POSTROUTING",
		"-p", protocol,
		"-s", containerIP,
		"--sport", strconv.Itoa(containerPort),
		"-j", "MASQUERADE",
	}
	exec.Command(masqRule[0], masqRule[1:]...).Run()

	return nil
}

// FindAvailablePort finds an available port on the host
func FindAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

// ValidatePort checks if a port is valid
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port %d is out of valid range (1-65535)", port)
	}
	return nil
}

// CheckPortAvailable checks if a port is available on the host
func CheckPortAvailable(port int) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}
