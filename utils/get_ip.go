package utils

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
)

func GetInternalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get network interfaces: %v", err)
	}

	for _, addr := range addrs {
		// Check if the address is an IP network address and not a loopback address
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil { // Return only IPv4 addresses
				return ipNet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no IP address found")
}

func GetExternalIP() (string, error) {
	// External service URL that returns the public IP
	resp, err := http.Get("https://ifconfig.me")
	if err != nil {
		return "", fmt.Errorf("failed to get external IP: %v", err)
	}
	defer resp.Body.Close()

	ip, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	return string(ip), nil
}
