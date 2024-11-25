package utils

import (
	"io/ioutil"
	"log"
	"net"
	"net/http"
)

// GetLocalIP returns the local IP address of the node
func GetLocalIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Fatalf("Failed to get network interfaces: %v", err)
	}

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			log.Printf("Failed to get addresses for interface %s: %v", iface.Name, err)
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
				// Return the first non-loopback IPv4 address
				return ipNet.IP.String()
			}
		}
	}
	return "0.0.0.0" // Default fallback if no IP found
}

// GetPublicIP returns the public IP address of the node by querying an external service
func GetPublicIP() string {
	resp, err := http.Get("http://checkip.amazonaws.com/")
	if err != nil {
		log.Printf("Failed to get public IP: %v", err)
		return "0.0.0.0"
	}
	defer resp.Body.Close()

	publicIP, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read public IP response: %v", err)
		return "0.0.0.0"
	}

	return string(publicIP)
}
