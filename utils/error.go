package utils

import "strings"

func IsReplacementError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "nonce too low") || strings.Contains(err.Error(), "replacement transaction underpriced") || strings.Contains(err.Error(), "already known")
}
