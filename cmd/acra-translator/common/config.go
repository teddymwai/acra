/*
Copyright 2018, Cossack Labs Limited

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"crypto/tls"
	"github.com/cossacklabs/acra/network"
	"github.com/cossacklabs/acra/pseudonymization/common"
	"go.opencensus.io/trace"
)

// AcraTranslatorConfig stores keys, poison record settings, connection attributes.
type AcraTranslatorConfig struct {
	keysDir                      string
	detectPoisonRecords          bool
	scriptOnPoison               string
	stopOnPoison                 bool
	serverID                     []byte
	incomingConnectionHTTPString string
	incomingConnectionGRPCString string
	HTTPConnectionWrapper        network.HTTPServerConnectionWrapper
	GRPCConnectionWrapper        network.GRPCConnectionWrapper
	configPath                   string
	debug                        bool
	traceToLog                   bool
	tlsConfig                    *tls.Config
	useClientIDFromConnection    bool
	withConnector                bool
	tokenizer                    common.Pseudoanonymizer
	tlsClientIDExtractor         network.TLSClientIDExtractor
}

// NewConfig creates new AcraTranslatorConfig.
func NewConfig() *AcraTranslatorConfig {
	return &AcraTranslatorConfig{stopOnPoison: false}
}

// SetWithConnector set WithConnector
func (a *AcraTranslatorConfig) SetWithConnector(v bool) {
	a.withConnector = v
}

// SetTLSClientIDExtractor set clientID extractor from TLS metadata
func (a *AcraTranslatorConfig) SetTLSClientIDExtractor(tlsClientIDExtractor network.TLSClientIDExtractor) {
	a.tlsClientIDExtractor = tlsClientIDExtractor
}

// GetTLSClientIDExtractor return configured TLSClietIDExtractor
func (a *AcraTranslatorConfig) GetTLSClientIDExtractor() network.TLSClientIDExtractor {
	return a.tlsClientIDExtractor
}

// SetTokenizer set configured tokenizer
func (a *AcraTranslatorConfig) SetTokenizer(tokenizer common.Pseudoanonymizer) {
	a.tokenizer = tokenizer
}

// GetTokenizer return configure tokenizer
func (a *AcraTranslatorConfig) GetTokenizer() common.Pseudoanonymizer {
	return a.tokenizer
}

// GetWithConnector return WithConnector
func (a *AcraTranslatorConfig) GetWithConnector() bool {
	return a.withConnector
}

// SetUseClientIDFromConnection use ClientID from connection metadata instead request arguments
func (a *AcraTranslatorConfig) SetUseClientIDFromConnection(v bool) {
	a.useClientIDFromConnection = v
}

// GetUseClientIDFromConnection return true if translator should use clientID from connection
func (a *AcraTranslatorConfig) GetUseClientIDFromConnection() bool {
	return a.useClientIDFromConnection
}

// WithTLS true if server should use TLS connections to gRPC/HTTP server
func (a *AcraTranslatorConfig) WithTLS() bool {
	return a.tlsConfig != nil
}

// SetTLSConfig tls.Config which should be used
func (a *AcraTranslatorConfig) SetTLSConfig(v *tls.Config) {
	a.tlsConfig = v
}

// GetTLSConfig return tls.Config which should be used
func (a *AcraTranslatorConfig) GetTLSConfig() *tls.Config {
	return a.tlsConfig
}

// SetTraceToLog true if want to log trace data otherwise false
func (a *AcraTranslatorConfig) SetTraceToLog(v bool) {
	a.traceToLog = v
}

// GetTraceOptions for opencensus trace
func (a *AcraTranslatorConfig) GetTraceOptions() []trace.StartOption {
	return []trace.StartOption{trace.WithSampler(trace.AlwaysSample()), trace.WithSpanKind(trace.SpanKindServer)}
}

// KeysDir returns keys directory.
func (a *AcraTranslatorConfig) KeysDir() string {
	return a.keysDir
}

// SetKeysDir sets keys directory.
func (a *AcraTranslatorConfig) SetKeysDir(keysDir string) {
	a.keysDir = keysDir
}

// SetDetectPoisonRecords sets if AcraTranslator should detect poison records.
func (a *AcraTranslatorConfig) SetDetectPoisonRecords(val bool) {
	a.detectPoisonRecords = val
}

// DetectPoisonRecords returns if AcraTranslator should detect poison records.
func (a *AcraTranslatorConfig) DetectPoisonRecords() bool {
	return a.detectPoisonRecords
}

// ScriptOnPoison returns script-to-run on detection of poison records.
func (a *AcraTranslatorConfig) ScriptOnPoison() string {
	return a.scriptOnPoison
}

// SetScriptOnPoison sets script-to-run on detection of poison records.
func (a *AcraTranslatorConfig) SetScriptOnPoison(scriptOnPoison string) {
	a.scriptOnPoison = scriptOnPoison
}

// StopOnPoison returns if AcraTranslator should stop working on detection of poison records.
func (a *AcraTranslatorConfig) StopOnPoison() bool {
	return a.stopOnPoison
}

// SetStopOnPoison sets if AcraTranslator should stop working on detection of poison records.
func (a *AcraTranslatorConfig) SetStopOnPoison(stopOnPoison bool) {
	a.stopOnPoison = stopOnPoison
}

// ServerID returns server id associated with SecureSession connection.
func (a *AcraTranslatorConfig) ServerID() []byte {
	return a.serverID
}

// SetServerID sets server id associated with SecureSession connection.
func (a *AcraTranslatorConfig) SetServerID(serverID []byte) {
	a.serverID = serverID
}

// IncomingConnectionHTTPString returns connection string to listen for HTTP requests.
func (a *AcraTranslatorConfig) IncomingConnectionHTTPString() string {
	return a.incomingConnectionHTTPString
}

// SetIncomingConnectionHTTPString sets connection string to listen for HTTP requests.
func (a *AcraTranslatorConfig) SetIncomingConnectionHTTPString(incomingConnectionHTTPString string) {
	a.incomingConnectionHTTPString = incomingConnectionHTTPString
}

// IncomingConnectionGRPCString returns connection string to listen for gRPC requests.
func (a *AcraTranslatorConfig) IncomingConnectionGRPCString() string {
	return a.incomingConnectionGRPCString
}

// SetIncomingConnectionGRPCString sets connection string to listen for gRPC requests.
func (a *AcraTranslatorConfig) SetIncomingConnectionGRPCString(incomingConnectionGRPCString string) {
	a.incomingConnectionGRPCString = incomingConnectionGRPCString
}

// ConfigPath returns configuration path for AcraTranslator.
func (a *AcraTranslatorConfig) ConfigPath() string {
	return a.configPath
}

// SetConfigPath sets configuration path for AcraTranslator.
func (a *AcraTranslatorConfig) SetConfigPath(configPath string) {
	a.configPath = configPath
}

// Debug returns if should print debug logs.
func (a *AcraTranslatorConfig) Debug() bool {
	return a.debug
}

// SetDebug sets if should print debug logs.
func (a *AcraTranslatorConfig) SetDebug(debug bool) {
	a.debug = debug
}
