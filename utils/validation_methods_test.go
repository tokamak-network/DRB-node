package utils

import (
	"strings"
	"testing"
)

func TestRegistrationRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     RegistrationRequest
		wantErr bool
		errCode string
	}{
		{
			name: "valid registration request",
			req: RegistrationRequest{
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
				PeerID:     "12D3KooWBmwkafWE",
			},
			wantErr: false,
		},
		{
			name: "empty eoa address",
			req: RegistrationRequest{
				EOAAddress: "",
				Signature:  make([]byte, 65),
				PeerID:     "12D3KooWBmwkafWE",
			},
			wantErr: true,
			errCode: "REQUIRED",
		},
		{
			name: "invalid eoa address format",
			req: RegistrationRequest{
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE1",
				Signature:  make([]byte, 65),
				PeerID:     "12D3KooWBmwkafWE",
			},
			wantErr: true,
			errCode: "INVALID_LENGTH",
		},
		{
			name: "empty signature",
			req: RegistrationRequest{
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  []byte{},
				PeerID:     "12D3KooWBmwkafWE",
			},
			wantErr: true,
			errCode: "REQUIRED",
		},
		{
			name: "invalid signature length",
			req: RegistrationRequest{
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 64),
				PeerID:     "12D3KooWBmwkafWE",
			},
			wantErr: true,
			errCode: "INVALID_SIGNATURE_LENGTH",
		},
		{
			name: "empty peer id",
			req: RegistrationRequest{
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
				PeerID:     "",
			},
			wantErr: true,
			errCode: "REQUIRED",
		},
		{
			name: "peer id empty but not required anymore due to maxLen removal",
			req: RegistrationRequest{
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
				PeerID:     strings.Repeat("a", 1000), // This should now pass since maxLen was removed
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("RegistrationRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				if validationErr, ok := err.(*ValidationError); ok {
					if validationErr.Code != tt.errCode {
						t.Errorf("RegistrationRequest.Validate() error code = %v, want %v", validationErr.Code, tt.errCode)
					}
				} else {
					t.Errorf("Expected ValidationError, got %T", err)
				}
			}
		})
	}
}

func TestSecretValueRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     SecretValueRequest
		wantErr bool
		errCode string
	}{
		{
			name: "valid secret value request",
			req: SecretValueRequest{
				LeaderEoaAddress:  "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				RegularEoaAddress: "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Round:             "123",
				TrialNum:          "1",
				Signature:         make([]byte, 65),
				SecretValue:       make([]byte, 32),
				Order:             5,
			},
			wantErr: false,
		},
		{
			name: "secret value too long",
			req: SecretValueRequest{
				LeaderEoaAddress:  "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				RegularEoaAddress: "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Round:             "123",
				TrialNum:          "1",
				Signature:         make([]byte, 65),
				SecretValue:       make([]byte, 33),
				Order:             5,
			},
			wantErr: true,
			errCode: "TOO_LONG",
		},
		{
			name: "empty leader eoa address",
			req: SecretValueRequest{
				LeaderEoaAddress:  "",
				RegularEoaAddress: "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Round:             "123",
				TrialNum:          "1",
				Signature:         make([]byte, 65),
				SecretValue:       make([]byte, 32),
				Order:             5,
			},
			wantErr: true,
			errCode: "REQUIRED",
		},
		{
			name: "empty regular eoa address",
			req: SecretValueRequest{
				LeaderEoaAddress:  "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				RegularEoaAddress: "",
				Round:             "123",
				TrialNum:          "1",
				Signature:         make([]byte, 65),
				SecretValue:       make([]byte, 32),
				Order:             5,
			},
			wantErr: true,
			errCode: "REQUIRED",
		},
		{
			name: "empty round",
			req: SecretValueRequest{
				LeaderEoaAddress:  "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				RegularEoaAddress: "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Round:             "",
				TrialNum:          "1",
				Signature:         make([]byte, 65),
				SecretValue:       make([]byte, 32),
				Order:             5,
			},
			wantErr: true,
			errCode: "REQUIRED",
		},
		{
			name: "non-numeric round",
			req: SecretValueRequest{
				LeaderEoaAddress:  "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				RegularEoaAddress: "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Round:             "abc",
				TrialNum:          "1",
				Signature:         make([]byte, 65),
				SecretValue:       make([]byte, 32),
				Order:             5,
			},
			wantErr: true,
			errCode: "INVALID_NUMERIC",
		},
		{
			name: "empty trial num",
			req: SecretValueRequest{
				LeaderEoaAddress:  "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				RegularEoaAddress: "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Round:             "123",
				TrialNum:          "",
				Signature:         make([]byte, 65),
				SecretValue:       make([]byte, 32),
				Order:             5,
			},
			wantErr: true,
			errCode: "REQUIRED",
		},
		{
			name: "non-numeric trial num",
			req: SecretValueRequest{
				LeaderEoaAddress:  "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				RegularEoaAddress: "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Round:             "123",
				TrialNum:          "abc",
				Signature:         make([]byte, 65),
				SecretValue:       make([]byte, 32),
				Order:             5,
			},
			wantErr: true,
			errCode: "INVALID_NUMERIC",
		},
		{
			name: "invalid signature length",
			req: SecretValueRequest{
				LeaderEoaAddress:  "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				RegularEoaAddress: "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Round:             "123",
				TrialNum:          "1",
				Signature:         make([]byte, 64),
				SecretValue:       make([]byte, 32),
				Order:             5,
			},
			wantErr: true,
			errCode: "INVALID_SIGNATURE_LENGTH",
		},
		{
			name: "invalid secret value length",
			req: SecretValueRequest{
				LeaderEoaAddress:  "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				RegularEoaAddress: "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Round:             "123",
				TrialNum:          "1",
				Signature:         make([]byte, 65),
				SecretValue:       make([]byte, 31),
				Order:             5,
			},
			wantErr: true,
			errCode: "INVALID_LENGTH",
		},
		{
			name: "order too small (negative)",
			req: SecretValueRequest{
				LeaderEoaAddress:  "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				RegularEoaAddress: "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Round:             "123",
				TrialNum:          "1",
				Signature:         make([]byte, 65),
				SecretValue:       make([]byte, 32),
				Order:             -1,
			},
			wantErr: true,
			errCode: "TOO_SMALL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SecretValueRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				if validationErr, ok := err.(*ValidationError); ok {
					if validationErr.Code != tt.errCode {
						t.Errorf("SecretValueRequest.Validate() error code = %v, want %v", validationErr.Code, tt.errCode)
					}
				} else {
					t.Errorf("Expected ValidationError, got %T", err)
				}
			}
		})
	}
}

func TestCommitRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     CommitRequest
		wantErr bool
		errCode string
	}{
		{
			name: "valid commit request",
			req: CommitRequest{
				UniqueKey:  "123-1",
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
				Sign: SignInfo{
					R: strings.Repeat("a", 64),
					S: strings.Repeat("b", 64),
					V: "1c",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid sign info R component",
			req: CommitRequest{
				UniqueKey:  "123-1",
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
				Sign: SignInfo{
					R: strings.Repeat("a", 63), // Invalid length
					S: strings.Repeat("b", 64),
					V: "1c",
				},
			},
			wantErr: true,
			errCode: "INVALID_LENGTH",
		},
		{
			name: "non-numeric round",
			req: CommitRequest{
				UniqueKey:  "123-1",
				Round:      "abc",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
				Sign: SignInfo{
					R: strings.Repeat("a", 64),
					S: strings.Repeat("b", 64),
					V: "1c",
				},
			},
			wantErr: true,
			errCode: "INVALID_NUMERIC",
		},
		{
			name: "non-numeric trial num",
			req: CommitRequest{
				UniqueKey:  "123-1",
				Round:      "123",
				TrialNum:   "xyz",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
				Sign: SignInfo{
					R: strings.Repeat("a", 64),
					S: strings.Repeat("b", 64),
					V: "1c",
				},
			},
			wantErr: true,
			errCode: "INVALID_NUMERIC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("CommitRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				if validationErr, ok := err.(*ValidationError); ok {
					if validationErr.Code != tt.errCode {
						t.Errorf("CommitRequest.Validate() error code = %v, want %v", validationErr.Code, tt.errCode)
					}
				} else {
					t.Errorf("Expected ValidationError, got %T", err)
				}
			}
		})
	}
}

func TestCosRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     CosRequest
		wantErr bool
		errCode string
	}{
		{
			name: "valid cos request",
			req: CosRequest{
				UniqueKey:  "123-1",
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
			},
			wantErr: false,
		},
		{
			name: "invalid eoa address",
			req: CosRequest{
				UniqueKey:  "123-1",
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "invalid",
				Signature:  make([]byte, 65),
			},
			wantErr: true,
			errCode: "INVALID_LENGTH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("CosRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				if validationErr, ok := err.(*ValidationError); ok {
					if validationErr.Code != tt.errCode {
						t.Errorf("CosRequest.Validate() error code = %v, want %v", validationErr.Code, tt.errCode)
					}
				} else {
					t.Errorf("Expected ValidationError, got %T", err)
				}
			}
		})
	}
}

func TestBroadcastMessage_Validate(t *testing.T) {
	tests := []struct {
		name    string
		msg     BroadcastMessage
		wantErr bool
		errCode string
	}{
		{
			name: "valid broadcast message - cvs",
			msg: BroadcastMessage{
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "msg-123-1",
				Type:       "cvs",
				SignerEOA:  "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
			},
			wantErr: false,
		},
		{
			name: "valid broadcast message - cos",
			msg: BroadcastMessage{
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "msg-123-1",
				Type:       "cos",
				SignerEOA:  "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
			},
			wantErr: false,
		},
		{
			name: "valid broadcast message - secret",
			msg: BroadcastMessage{
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "msg-123-1",
				Type:       "secret",
				SignerEOA:  "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
			},
			wantErr: false,
		},
		{
			name: "invalid type",
			msg: BroadcastMessage{
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "msg-123-1",
				Type:       "invalid",
				SignerEOA:  "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
			},
			wantErr: true,
			errCode: "INVALID_TYPE",
		},
		{
			name: "empty message id",
			msg: BroadcastMessage{
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "",
				Type:       "cvs",
				SignerEOA:  "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
			},
			wantErr: true,
			errCode: "REQUIRED",
		},
		{
			name: "invalid signer eoa",
			msg: BroadcastMessage{
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "msg-123-1",
				Type:       "cvs",
				SignerEOA:  "invalid",
				Signature:  make([]byte, 65),
			},
			wantErr: true,
			errCode: "INVALID_LENGTH",
		},
		{
			name: "empty round",
			msg: BroadcastMessage{
				Round:      "",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "msg-123-1",
				Type:       "cvs",
				SignerEOA:  "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
			},
			wantErr: true,
			errCode: "REQUIRED",
		},
		{
			name: "non-numeric round",
			msg: BroadcastMessage{
				Round:      "abc",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "msg-123-1",
				Type:       "cvs",
				SignerEOA:  "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
			},
			wantErr: true,
			errCode: "INVALID_NUMERIC",
		},
		{
			name: "non-numeric trial num",
			msg: BroadcastMessage{
				Round:      "123",
				TrialNum:   "xyz",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "msg-123-1",
				Type:       "cvs",
				SignerEOA:  "0x123d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				Signature:  make([]byte, 65),
			},
			wantErr: true,
			errCode: "INVALID_NUMERIC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("BroadcastMessage.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				if validationErr, ok := err.(*ValidationError); ok {
					if validationErr.Code != tt.errCode {
						t.Errorf("BroadcastMessage.Validate() error code = %v, want %v", validationErr.Code, tt.errCode)
					}
				} else {
					t.Errorf("Expected ValidationError, got %T", err)
				}
			}
		})
	}
}

func TestAcknowledgmentMessage_Validate(t *testing.T) {
	tests := []struct {
		name    string
		msg     AcknowledgmentMessage
		wantErr bool
		errCode string
	}{
		{
			name: "valid acknowledgment message - received",
			msg: AcknowledgmentMessage{
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "msg-123-1",
				Type:       "cvs",
				Status:     "received",
				Signature:  make([]byte, 65),
			},
			wantErr: false,
		},
		{
			name: "valid acknowledgment message - error",
			msg: AcknowledgmentMessage{
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "msg-123-1",
				Type:       "cos",
				Status:     "error",
				Signature:  make([]byte, 65),
			},
			wantErr: false,
		},
		{
			name: "invalid type",
			msg: AcknowledgmentMessage{
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "msg-123-1",
				Type:       "invalid",
				Status:     "received",
				Signature:  make([]byte, 65),
			},
			wantErr: true,
			errCode: "INVALID_TYPE",
		},
		{
			name: "invalid status",
			msg: AcknowledgmentMessage{
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "msg-123-1",
				Type:       "cvs",
				Status:     "invalid",
				Signature:  make([]byte, 65),
			},
			wantErr: true,
			errCode: "INVALID_STATUS",
		},
		{
			name: "empty round",
			msg: AcknowledgmentMessage{
				Round:      "",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "msg-123-1",
				Type:       "cvs",
				Status:     "received",
				Signature:  make([]byte, 65),
			},
			wantErr: true,
			errCode: "REQUIRED",
		},
		{
			name: "empty message id",
			msg: AcknowledgmentMessage{
				Round:      "123",
				TrialNum:   "1",
				EOAAddress: "0x742d35Cc6635C0532925a3b8D8129dAa1d5DaE14",
				MessageID:  "",
				Type:       "cvs",
				Status:     "received",
				Signature:  make([]byte, 65),
			},
			wantErr: true,
			errCode: "REQUIRED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("AcknowledgmentMessage.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				if validationErr, ok := err.(*ValidationError); ok {
					if validationErr.Code != tt.errCode {
						t.Errorf("AcknowledgmentMessage.Validate() error code = %v, want %v", validationErr.Code, tt.errCode)
					}
				} else {
					t.Errorf("Expected ValidationError, got %T", err)
				}
			}
		})
	}
}

func TestSignInfo_Validate(t *testing.T) {
	tests := []struct {
		name    string
		sign    SignInfo
		wantErr bool
		errCode string
	}{
		{
			name: "valid sign info",
			sign: SignInfo{
				R: strings.Repeat("a", 64),
				S: strings.Repeat("b", 64),
				V: "1c",
			},
			wantErr: false,
		},
		{
			name: "valid sign info with mixed case hex",
			sign: SignInfo{
				R: strings.Repeat("A", 32) + strings.Repeat("1", 32),
				S: strings.Repeat("B", 32) + strings.Repeat("2", 32),
				V: "1C",
			},
			wantErr: false,
		},
		{
			name: "empty R component",
			sign: SignInfo{
				R: "",
				S: strings.Repeat("b", 64),
				V: "1c",
			},
			wantErr: true,
			errCode: "REQUIRED",
		},
		{
			name: "invalid R component length",
			sign: SignInfo{
				R: strings.Repeat("a", 63),
				S: strings.Repeat("b", 64),
				V: "1c",
			},
			wantErr: true,
			errCode: "INVALID_LENGTH",
		},
		{
			name: "invalid R component hex",
			sign: SignInfo{
				R: strings.Repeat("z", 64),
				S: strings.Repeat("b", 64),
				V: "1c",
			},
			wantErr: true,
			errCode: "INVALID_HEX",
		},
		{
			name: "invalid S component length",
			sign: SignInfo{
				R: strings.Repeat("a", 64),
				S: strings.Repeat("b", 63),
				V: "1c",
			},
			wantErr: true,
			errCode: "INVALID_LENGTH",
		},
		{
			name: "invalid V component length",
			sign: SignInfo{
				R: strings.Repeat("a", 64),
				S: strings.Repeat("b", 64),
				V: "1",
			},
			wantErr: true,
			errCode: "INVALID_LENGTH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sign.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SignInfo.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				if validationErr, ok := err.(*ValidationError); ok {
					if validationErr.Code != tt.errCode {
						t.Errorf("SignInfo.Validate() error code = %v, want %v", validationErr.Code, tt.errCode)
					}
				} else {
					t.Errorf("Expected ValidationError, got %T", err)
				}
			}
		})
	}
}
