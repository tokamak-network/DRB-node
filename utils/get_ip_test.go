package utils

import (
	"testing"
)

// TestGetIP tests the GetIP function
func TestGetIP(t *testing.T) {
	//ip, err := GetInternalIP()
	//if err != nil {
	//	t.Fatalf("expected no error, but got %v", err)
	//}
	//
	//if ip == "" {
	//	t.Fatalf("expected a valid IP address, but got an empty string")
	//}
	//
	//t.Logf("IP address: %s", ip)

	ip, err := GetExternalIP()
	if err != nil {
		t.Fatalf("expected no error, but got %v", err)
	}

	if ip == "" {
		t.Fatalf("expected a valid IP address, but got an empty string")
	}

	t.Logf("IP address: %s", ip)
}
