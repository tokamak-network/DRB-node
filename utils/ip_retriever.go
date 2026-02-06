package utils

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

func GetLocalIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			log.Printf("Failed to get addresses for interface %s: %v", iface.Name, err)
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
				localIP := ipNet.IP.String()
				if net.ParseIP(localIP) == nil {
					continue
				}
				return localIP, nil
			}
		}
	}
	return "", fmt.Errorf("no valid non-loopback IPv4 address found on any network interface")
}
func GetPublicIP() (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get("https://checkip.amazonaws.com/")
	if err != nil {
		return "", fmt.Errorf("failed to get public IP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("public IP service returned non-200 status: %d", resp.StatusCode)
	}

	publicIPBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read public IP response: %w", err)
	}

	publicIP := strings.TrimSpace(string(publicIPBytes))
	if publicIP == "" {
		return "", fmt.Errorf("public IP service returned empty response")
	}
	if net.ParseIP(publicIP) == nil {
		return "", fmt.Errorf("public IP service returned invalid IP address: %q", publicIP)
	}

	return publicIP, nil
}
