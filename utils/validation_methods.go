package utils

// ValidateRegistrationRequest validates a RegistrationRequest struct
func (r *RegistrationRequest) Validate() error {
	if err := ValidateEOAAddress(r.EOAAddress, "eoa_address", true); err != nil {
		return err
	}

	if err := ValidateSignature(r.Signature, "signature"); err != nil {
		return err
	}

	if err := ValidateString(r.PeerID, "peer_id", true); err != nil {
		return err
	}

	return nil
}

// ValidateSecretValueRequest validates a SecretValueRequest struct
func (r *SecretValueRequest) Validate() error {
	if err := ValidateEOAAddress(r.LeaderEoaAddress, "leader_eoa", true); err != nil {
		return err
	}

	if err := ValidateEOAAddress(r.RegularEoaAddress, "regular_eoa", true); err != nil {
		return err
	}

	if err := ValidateNumericString(r.Round, "round", true); err != nil {
		return err
	}

	if err := ValidateNumericString(r.TrialNum, "trial_num", true); err != nil {
		return err
	}

	if err := ValidateSignature(r.Signature, "signature"); err != nil {
		return err
	}

	if err := ValidateByteArray(r.SecretValue, "secret_value", true, 32); err != nil {
		return err
	}

	// Validate that SecretValue is exactly 32 bytes for [32]byte arrays
	if len(r.SecretValue) != 32 {
		return &ValidationError{
			Field:   "secret_value",
			Code:    "INVALID_LENGTH",
			Message: "Secret value must be exactly 32 bytes",
		}
	}

	if err := ValidateIntField(r.Order, "order"); err != nil {
		return err
	}

	return nil
}

// ValidateCommitRequest validates a CommitRequest struct
func (r *CommitRequest) Validate() error {
	if err := ValidateString(r.UniqueKey, "unique_key", true); err != nil {
		return err
	}

	if err := ValidateNumericString(r.Round, "round", true); err != nil {
		return err
	}

	if err := ValidateNumericString(r.TrialNum, "trial_num", true); err != nil {
		return err
	}

	if err := ValidateEOAAddress(r.EOAAddress, "eoa_address", true); err != nil {
		return err
	}

	if err := ValidateSignature(r.Signature, "signed_round"); err != nil {
		return err
	}

	if err := r.Sign.Validate(); err != nil {
		return err
	}

	return nil
}

// ValidateCosRequest validates a CosRequest struct
func (r *CosRequest) Validate() error {
	if err := ValidateString(r.UniqueKey, "unique_key", true); err != nil {
		return err
	}

	if err := ValidateNumericString(r.Round, "round", true); err != nil {
		return err
	}

	if err := ValidateNumericString(r.TrialNum, "trial_num", true); err != nil {
		return err
	}

	if err := ValidateEOAAddress(r.EOAAddress, "eoa_address", true); err != nil {
		return err
	}

	if err := ValidateSignature(r.Signature, "signed_round"); err != nil {
		return err
	}

	return nil
}

// ValidateBroadcastMessage validates a BroadcastMessage struct
func (b *BroadcastMessage) Validate() error {
	if err := ValidateNumericString(b.Round, "round", true); err != nil {
		return err
	}

	if err := ValidateNumericString(b.TrialNum, "trial_num", true); err != nil {
		return err
	}

	if err := ValidateEOAAddress(b.EOAAddress, "eoa_address", true); err != nil {
		return err
	}

	if err := ValidateString(b.MessageID, "message_id", true); err != nil {
		return err
	}

	// Validate message type
	validTypes := map[string]bool{
		"cvs":    true,
		"cos":    true,
		"secret": true,
	}
	if !validTypes[b.Type] {
		return &ValidationError{
			Field:   "type",
			Code:    "INVALID_TYPE",
			Message: "Type must be one of: cvs, cos, secret",
		}
	}

	if err := ValidateEOAAddress(b.SignerEOA, "signer_eoa", true); err != nil {
		return err
	}

	if err := ValidateSignature(b.Signature, "signature"); err != nil {
		return err
	}

	return nil
}

// ValidateAcknowledgmentMessage validates an AcknowledgmentMessage struct
func (a *AcknowledgmentMessage) Validate() error {
	if err := ValidateNumericString(a.Round, "round", true); err != nil {
		return err
	}

	if err := ValidateNumericString(a.TrialNum, "trial_num", true); err != nil {
		return err
	}

	if err := ValidateEOAAddress(a.EOAAddress, "eoa_address", true); err != nil {
		return err
	}

	if err := ValidateString(a.MessageID, "message_id", true); err != nil {
		return err
	}

	// Validate message type
	validTypes := map[string]bool{
		"cvs":    true,
		"cos":    true,
		"secret": true,
	}
	if !validTypes[a.Type] {
		return &ValidationError{
			Field:   "type",
			Code:    "INVALID_TYPE",
			Message: "Type must be one of: cvs, cos, secret",
		}
	}

	// Validate status
	validStatuses := map[string]bool{
		"received": true,
		"error":    true,
	}
	if !validStatuses[a.Status] {
		return &ValidationError{
			Field:   "status",
			Code:    "INVALID_STATUS",
			Message: "Status must be one of: received, error",
		}
	}

	if err := ValidateSignature(a.Signature, "signature"); err != nil {
		return err
	}

	return nil
}

// ValidateSignInfo validates a SignInfo struct
func (s *SignInfo) Validate() error {
	if err := ValidateHexString(s.R, "r", true); err != nil {
		return err
	}

	if err := ValidateHexString(s.S, "s", true); err != nil {
		return err
	}

	if err := ValidateHexString(s.V, "v", true); err != nil {
		return err
	}

	// Additional validation for signature components
	if len(s.R) != 64 { // 32 bytes = 64 hex chars
		return &ValidationError{
			Field:   "r",
			Code:    "INVALID_LENGTH",
			Message: "R component must be 64 hex characters",
		}
	}

	if len(s.S) != 64 { // 32 bytes = 64 hex chars
		return &ValidationError{
			Field:   "s",
			Code:    "INVALID_LENGTH",
			Message: "S component must be 64 hex characters",
		}
	}

	if len(s.V) != 2 { // 1 byte = 2 hex chars
		return &ValidationError{
			Field:   "v",
			Code:    "INVALID_LENGTH",
			Message: "V component must be 2 hex characters",
		}
	}

	return nil
}
