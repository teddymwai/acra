/*
 * Copyright 2021, Cossack Labs Limited
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package config

import (
	"errors"
	maskingCommon "github.com/cossacklabs/acra/masking/common"
	"github.com/cossacklabs/acra/pseudonymization/common"
)

// SettingMask bitmask used to store info about encryptor configuration
type SettingMask int32

// set of flags according to BasicColumnEncryptionSetting public fields except Name which is required
const (
	SettingReEncryptionFlag SettingMask = 1 << iota
	SettingMaskingFlag
	SettingMaskingPlaintextLengthFlag
	SettingMaskingPlaintextSideFlag
	SettingTokenizationFlag
	SettingConsistentTokenizationFlag
	SettingTokenTypeFlag
	SettingSearchFlag
	SettingClientIDFlag
	SettingZoneIDFlag
	SettingAcraBlockEncryptionFlag
	SettingAcraStructEncryptionFlag
)

// validSettings store all valid combinations of encryption settings
var validSettings = map[SettingMask]struct{}{
	// JUST ENCRYPTION
	// reencrypt to acrablock

	// ClientID
	SettingClientIDFlag | SettingAcraStructEncryptionFlag: {},
	SettingClientIDFlag | SettingAcraBlockEncryptionFlag:  {},

	SettingClientIDFlag | SettingReEncryptionFlag | SettingAcraStructEncryptionFlag: {},
	SettingClientIDFlag | SettingReEncryptionFlag | SettingAcraBlockEncryptionFlag:  {},
	// ZoneID
	SettingZoneIDFlag | SettingAcraBlockEncryptionFlag:  {},
	SettingZoneIDFlag | SettingAcraStructEncryptionFlag: {},

	SettingZoneIDFlag | SettingAcraBlockEncryptionFlag | SettingReEncryptionFlag:  {},
	SettingZoneIDFlag | SettingAcraStructEncryptionFlag | SettingReEncryptionFlag: {},
	/////////////
	// SEARCHABLE
	// default ClientID
	SettingSearchFlag | SettingAcraStructEncryptionFlag | SettingReEncryptionFlag: {},
	SettingSearchFlag | SettingAcraBlockEncryptionFlag | SettingReEncryptionFlag:  {},
	// specified ClientID
	SettingSearchFlag | SettingClientIDFlag | SettingAcraStructEncryptionFlag | SettingReEncryptionFlag: {},
	SettingSearchFlag | SettingClientIDFlag | SettingAcraBlockEncryptionFlag | SettingReEncryptionFlag:  {},

	/////////////
	// MASKING (should be specified all 3 parameters)
	// default ClientID
	SettingAcraStructEncryptionFlag | SettingMaskingFlag | SettingMaskingPlaintextSideFlag | SettingMaskingPlaintextLengthFlag | SettingReEncryptionFlag: {},
	SettingAcraBlockEncryptionFlag | SettingMaskingFlag | SettingMaskingPlaintextSideFlag | SettingMaskingPlaintextLengthFlag | SettingReEncryptionFlag:  {},
	// specified ClientID
	SettingAcraStructEncryptionFlag | SettingMaskingFlag | SettingMaskingPlaintextSideFlag | SettingMaskingPlaintextLengthFlag | SettingClientIDFlag | SettingReEncryptionFlag: {},
	SettingAcraBlockEncryptionFlag | SettingMaskingFlag | SettingMaskingPlaintextSideFlag | SettingMaskingPlaintextLengthFlag | SettingClientIDFlag | SettingReEncryptionFlag:  {},
	// specified ZoneID
	SettingAcraStructEncryptionFlag | SettingMaskingFlag | SettingMaskingPlaintextSideFlag | SettingMaskingPlaintextLengthFlag | SettingZoneIDFlag | SettingReEncryptionFlag: {},
	SettingAcraBlockEncryptionFlag | SettingMaskingFlag | SettingMaskingPlaintextSideFlag | SettingMaskingPlaintextLengthFlag | SettingZoneIDFlag | SettingReEncryptionFlag:  {},

	/////////////
	// TOKENIZATION
	// default clientID
	SettingTokenizationFlag | SettingTokenTypeFlag | SettingReEncryptionFlag | SettingAcraBlockEncryptionFlag: {},

	SettingTokenizationFlag | SettingTokenTypeFlag | SettingConsistentTokenizationFlag | SettingReEncryptionFlag:                                  {},
	SettingTokenizationFlag | SettingTokenTypeFlag | SettingConsistentTokenizationFlag | SettingReEncryptionFlag | SettingAcraBlockEncryptionFlag: {},
	// specified clientID
	SettingTokenizationFlag | SettingTokenTypeFlag | SettingClientIDFlag | SettingReEncryptionFlag | SettingAcraBlockEncryptionFlag:                                     {},
	SettingTokenizationFlag | SettingTokenTypeFlag | SettingConsistentTokenizationFlag | SettingClientIDFlag | SettingReEncryptionFlag | SettingAcraBlockEncryptionFlag: {},
	// specified zoneID
	SettingTokenizationFlag | SettingTokenTypeFlag | SettingZoneIDFlag | SettingReEncryptionFlag | SettingAcraBlockEncryptionFlag:                                     {},
	SettingTokenizationFlag | SettingTokenTypeFlag | SettingConsistentTokenizationFlag | SettingZoneIDFlag | SettingReEncryptionFlag | SettingAcraBlockEncryptionFlag: {},
}

// Token type names as expected in the configuration file.
var tokenTypeNames = map[string]common.TokenType{
	"int32": common.TokenType_Int32,
	"int64": common.TokenType_Int64,
	"str":   common.TokenType_String,
	"bytes": common.TokenType_Bytes,
	"email": common.TokenType_Email,

	"int32_string": common.TokenType_Int32Str,
	"int64_string": common.TokenType_Int64Str,
}

// CryptoEnvelopeType type of crypto envelope for encryptors
type CryptoEnvelopeType string

// Supported CryptoEnvelopeTypes
const (
	CryptoEnvelopeTypeAcraStruct CryptoEnvelopeType = "acrastruct"
	CryptoEnvelopeTypeAcraBlock  CryptoEnvelopeType = "acrablock"
)

// ErrInvalidCryptoEnvelopeType used for invalid values of CryptoEnvelopeType
var ErrInvalidCryptoEnvelopeType = errors.New("invalid CryptoEnvelopeType")

// ErrInvalidEncryptorConfig has invalid configuration
var ErrInvalidEncryptorConfig = errors.New("invalid encryptor config")

// ValidateCryptoEnvelopeType return error if value is unsupported CryptoEnvelopeType
func ValidateCryptoEnvelopeType(value CryptoEnvelopeType) error {
	switch value {
	case CryptoEnvelopeTypeAcraStruct, CryptoEnvelopeTypeAcraBlock:
		return nil
	default:
		return ErrInvalidCryptoEnvelopeType
	}
}

// BasicColumnEncryptionSetting is a basic set of column encryption settings.
type BasicColumnEncryptionSetting struct {
	Name         string `yaml:"column"`
	UsedClientID string `yaml:"client_id"`
	UsedZoneID   string `yaml:"zone_id"`

	// Data pseudonymization (tokenization)
	Tokenized              bool   `yaml:"tokenized"`
	ConsistentTokenization bool   `yaml:"consistent_tokenization"`
	TokenType              string `yaml:"token_type"`

	// Searchable encryption
	Searchable bool `yaml:"searchable"`
	// Data masking
	MaskingPattern           string                      `yaml:"masking"`
	PartialPlaintextLenBytes int                         `yaml:"plaintext_length"`
	PlaintextSide            maskingCommon.PlainTextSide `yaml:"plaintext_side"`
	CryptoEnvelope           *CryptoEnvelopeType         `yaml:"crypto_envelope"`
	ReEncryptToAcraBlock     *bool                       `yaml:"reencrypting_to_acrablocks"`
	settingMask              SettingMask
}

// Init validate and initialize SettingMask
func (s *BasicColumnEncryptionSetting) Init() error {
	if len(s.Name) == 0 {
		return ErrInvalidEncryptorConfig
	}

	s.settingMask = 0
	if len(s.ClientID()) > 0 {
		s.settingMask |= SettingClientIDFlag
	} else if len(s.ZoneID()) > 0 {
		s.settingMask |= SettingZoneIDFlag
	} else {
		// will be used default ClientID
		s.settingMask |= SettingClientIDFlag
	}
	if s.CryptoEnvelope != nil {
		if err := ValidateCryptoEnvelopeType(*s.CryptoEnvelope); err != nil {
			return err
		}
		switch *s.CryptoEnvelope {
		case CryptoEnvelopeTypeAcraStruct:
			s.settingMask |= SettingAcraStructEncryptionFlag
			break
		case CryptoEnvelopeTypeAcraBlock:
			s.settingMask |= SettingAcraBlockEncryptionFlag
			break
		}
	}
	if s.ReEncryptToAcraBlock != nil && *s.ReEncryptToAcraBlock {
		s.settingMask |= SettingReEncryptionFlag
	}

	if s.Tokenized {
		s.settingMask |= SettingTokenizationFlag
		tokenType, ok := tokenTypeNames[s.TokenType]
		if !ok {
			return common.ErrUnknownTokenType
		}
		if err := common.ValidateTokenType(tokenType); err != nil {
			return err
		}
		s.settingMask |= SettingTokenTypeFlag
		if s.ConsistentTokenization {
			s.settingMask |= SettingConsistentTokenizationFlag
		}
		// due to tokenization supports only AcraBlock and for backward compatibility, we reconfigure CryptoEnvelope always for AcraBlock
		// to leave Defaults support
		s.settingMask &= ^SettingAcraStructEncryptionFlag
		s.settingMask |= SettingAcraBlockEncryptionFlag
	}

	if s.MaskingPattern != "" || s.PlaintextSide != "" {
		if err := maskingCommon.ValidateMaskingParams(s.MaskingPattern, s.PartialPlaintextLenBytes, s.PlaintextSide); err != nil {
			return err
		}
		s.settingMask |= SettingMaskingFlag | SettingMaskingPlaintextLengthFlag | SettingMaskingPlaintextSideFlag
	}
	if s.Searchable {
		s.settingMask |= SettingSearchFlag
	}
	_, ok := validSettings[s.settingMask]
	if !ok {
		return ErrInvalidEncryptorConfig
	}
	return nil
}

// OnlyEncryption return true if should be applied only AcraStruct/AcraBlock encryption without tokenization/masking/etc
func (s *BasicColumnEncryptionSetting) OnlyEncryption() bool {
	return s.settingMask&(SettingMaskingFlag|SettingTokenizationFlag|SettingSearchFlag) == 0
}

// GetSettingMask return SettingMask
func (s *BasicColumnEncryptionSetting) GetSettingMask() SettingMask {
	return s.settingMask
}

// ColumnName returns name of the column for which these settings are for.
func (s *BasicColumnEncryptionSetting) ColumnName() string {
	return s.Name
}

// GetCryptoEnvelope returns type of crypto envelope
func (s *BasicColumnEncryptionSetting) GetCryptoEnvelope() CryptoEnvelopeType {
	if s.CryptoEnvelope == nil {
		return CryptoEnvelopeTypeAcraStruct
	}
	return *s.CryptoEnvelope
}

// ShouldReEncryptAcraStructToAcraBlock return true if should  re-encrypt data with AcraBlock
func (s *BasicColumnEncryptionSetting) ShouldReEncryptAcraStructToAcraBlock() bool {
	if s.ReEncryptToAcraBlock == nil {
		return false
	}
	return *s.ReEncryptToAcraBlock
}

// ClientID returns client ID to use when encrypting this column.
func (s *BasicColumnEncryptionSetting) ClientID() []byte {
	return []byte(s.UsedClientID)
}

// ZoneID returns zone ID to use when encrypting this column.
func (s *BasicColumnEncryptionSetting) ZoneID() []byte {
	return []byte(s.UsedZoneID)
}

// IsTokenized returns true if the column should be tokenized.
func (s *BasicColumnEncryptionSetting) IsTokenized() bool {
	return s.Tokenized
}

// IsConsistentTokenization returns true if column tokens should be consistent.
func (s *BasicColumnEncryptionSetting) IsConsistentTokenization() bool {
	return s.ConsistentTokenization
}

// GetTokenType return the type of tokenization to apply to the column.
func (s *BasicColumnEncryptionSetting) GetTokenType() common.TokenType {
	// If the configuration file contains some unknown or unsupported token type,
	// return some safe default.
	const defaultTokenType = common.TokenType_Bytes
	tokenType, ok := tokenTypeNames[s.TokenType]
	if !ok {
		return defaultTokenType
	}
	return common.NormalizeTokenType(tokenType, defaultTokenType)
}

// IsSearchable returns true if column should be searchable.
func (s *BasicColumnEncryptionSetting) IsSearchable() bool {
	return s.Searchable
}

// GetMaskingPattern returns string which should be used to mask AcraStruct data.
func (s *BasicColumnEncryptionSetting) GetMaskingPattern() string {
	return s.MaskingPattern
}

// GetPartialPlaintextLen returns number of bytes to be left untouched in masked value.
func (s *BasicColumnEncryptionSetting) GetPartialPlaintextLen() int {
	return s.PartialPlaintextLenBytes
}

// IsEndMasking returns true if the right part of the value should be masked.
func (s *BasicColumnEncryptionSetting) IsEndMasking() bool {
	return s.PlaintextSide == maskingCommon.PlainTextSideLeft
}

func (s *BasicColumnEncryptionSetting) applyDefaults(defaults defaultValues) {
	if s.CryptoEnvelope == nil {
		v := defaults.GetCryptoEnvelope()
		// not applicable to masking, tokenization and searchable encryption
		if s.settingMask&(SettingTokenizationFlag|SettingMaskingFlag) == 0 {
			s.CryptoEnvelope = &v
		}
	}
	if s.ReEncryptToAcraBlock == nil {
		v := defaults.ShouldReEncryptAcraStructToAcraBlock()
		// not applicable to masking, tokenization and searchable encryption
		if s.settingMask&(SettingTokenizationFlag|SettingMaskingFlag) == 0 {
			s.ReEncryptToAcraBlock = &v
		}
	}
}
