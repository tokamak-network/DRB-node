package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"unicode"
)

const (
	MaxEOALength    = 42 // Ethereum EOA addresses are 42 chars with 0x prefix
)

// ValidationError represents a JSON validation error
type ValidationError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (v ValidationError) Error() string {
	return fmt.Sprintf("validation error on field %s: %s", v.Field, v.Message)
}

// CreateSecureDecoder creates a JSON decoder with security settings
func CreateSecureDecoder(body io.Reader) *json.Decoder {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	return decoder
}

// ValidateStreamDecoding performs secure JSON decoding from stream
func ValidateStreamDecoding(stream io.Reader, target interface{}) error {
	decoder := CreateSecureDecoder(stream)
	
	if err := decoder.Decode(target); err != nil {
		return &ValidationError{
			Field:   "json",
			Code:    "DECODE_ERROR",
			Message: fmt.Sprintf("Failed to decode JSON from stream: %v", err),
		}
	}

	return nil
}

// ValidateString validates a string field with length constraints
func ValidateString(value, fieldName string, required bool) error {
	if required && value == "" {
		return &ValidationError{
			Field:   fieldName,
			Code:    "REQUIRED",
			Message: "Field is required",
		}
	}

	return nil
}

// ValidateEOAAddress validates an Ethereum EOA address
func ValidateEOAAddress(address, fieldName string, required bool) error {
	if required && address == "" {
		return &ValidationError{
			Field:   fieldName,
			Code:    "REQUIRED",
			Message: "EOA address is required",
		}
	}

	if address == "" && !required {
		return nil
	}

	if len(address) != MaxEOALength {
		return &ValidationError{
			Field:   fieldName,
			Code:    "INVALID_LENGTH",
			Message: fmt.Sprintf("EOA address must be exactly %d characters", MaxEOALength),
		}
	}

	if address[:2] != "0x" {
		return &ValidationError{
			Field:   fieldName,
			Code:    "INVALID_PREFIX",
			Message: "EOA address must start with 0x",
		}
	}

	// Validate hex characters
	for _, r := range address[2:] {
		if !unicode.Is(unicode.ASCII_Hex_Digit, r) {
			return &ValidationError{
				Field:   fieldName,
				Code:    "INVALID_HEX",
				Message: "EOA address contains invalid hexadecimal characters",
			}
		}
	}

	return nil
}

// ValidateNumericString validates a string that should contain only digits
func ValidateNumericString(value, fieldName string, required bool) error {
	if required && value == "" {
		return &ValidationError{
			Field:   fieldName,
			Code:    "REQUIRED",
			Message: "Numeric field is required",
		}
	}

	if value == "" && !required {
		return nil
	}

	// Check if all characters are digits
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return &ValidationError{
				Field:   fieldName,
				Code:    "INVALID_NUMERIC",
				Message: "Field must contain only digits",
			}
		}
	}

	// Additional check for reasonable range (prevent extremely large numbers)
	if len(value) > 20 { // Max uint64 is ~19 digits
		return &ValidationError{
			Field:   fieldName,
			Code:    "NUMBER_TOO_LARGE",
			Message: "Numeric value exceeds maximum allowed size",
		}
	}

	return nil
}

// ValidateIntField validates an integer field
func ValidateIntField(value int, fieldName string) error {
	if value < 0 {
		return &ValidationError{
			Field:   fieldName,
			Code:    "TOO_SMALL",
			Message: "Value must be at least 0",
		}
	}
	return nil
}

// ValidateByteArray validates a byte array field
func ValidateByteArray(value []byte, fieldName string, required bool, maxLen int) error {
	if required && len(value) == 0 {
		return &ValidationError{
			Field:   fieldName,
			Code:    "REQUIRED",
			Message: "Byte array is required",
		}
	}

	if len(value) > maxLen {
		return &ValidationError{
			Field:   fieldName,
			Code:    "TOO_LONG",
			Message: fmt.Sprintf("Byte array exceeds maximum length of %d", maxLen),
		}
	}

	return nil
}

// ValidateSignature validates a signature field
func ValidateSignature(signature []byte, fieldName string) error {
	if err := ValidateByteArray(signature, fieldName, true, 65); err != nil {
		return err
	}

	// Ethereum signatures should be exactly 65 bytes
	if len(signature) != 65 {
		return &ValidationError{
			Field:   fieldName,
			Code:    "INVALID_SIGNATURE_LENGTH",
			Message: "Signature must be exactly 65 bytes",
		}
	}

	return nil
}

// ValidateHexString validates a hex string
func ValidateHexString(value, fieldName string, required bool) error {
	if required && value == "" {
		return &ValidationError{
			Field:   fieldName,
			Code:    "REQUIRED",
			Message: "Hex string is required",
		}
	}

	if value == "" && !required {
		return nil
	}

	// Check if it's valid hex
	for _, r := range value {
		if !unicode.Is(unicode.ASCII_Hex_Digit, r) {
			return &ValidationError{
				Field:   fieldName,
				Code:    "INVALID_HEX",
				Message: "String contains invalid hexadecimal characters",
			}
		}
	}

	return nil
}