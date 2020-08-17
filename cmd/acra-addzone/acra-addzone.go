/*
Copyright 2016, Cossack Labs Limited

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

// Package main is entry point for AcraAddZone utility. AcraAddZone allows to generate Zone data (ID and public key)
// that should be used to create AcraStructs.
// Zones are the way to cryptographically compartmentalise records in an already-encrypted environment.
// Zones rely on different private keys on the server side. The idea behind Zones is very simple
// (yet quite specific to some use-cases): when we store sensitive data, it's frequently related to users /
// companies / some other binding entities. These entities could be described through some real-world identifiers,
// or (preferably) random identifiers, which have no computable relationship to the protected data.
// Acra uses this identifier to also identify, which key to use for decryption of a corresponding AcraStruct.
//
// https://github.com/cossacklabs/acra/wiki/Zones
// https://github.com/cossacklabs/acra/wiki/AcraConnector-and-AcraWriter#client-side-with-zones
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cossacklabs/acra/cmd"
	"github.com/cossacklabs/acra/keystore"
	"github.com/cossacklabs/acra/keystore/filesystem"
	keystoreV2 "github.com/cossacklabs/acra/keystore/v2/keystore"
	filesystemV2 "github.com/cossacklabs/acra/keystore/v2/keystore/filesystem"
	"github.com/cossacklabs/acra/logging"
	"github.com/cossacklabs/acra/utils"
	"github.com/cossacklabs/acra/zone"
	"github.com/cossacklabs/themis/gothemis/keys"
	log "github.com/sirupsen/logrus"
)

// Constants used by AcraAddZone util.
var (
	// defaultConfigPath relative path to config which will be parsed as default
	defaultConfigPath = utils.GetConfigPathByName("acra-addzone")
	serviceName       = "acra-addzone"
)

func main() {
	outputDir := flag.String("keys_output_dir", keystore.DefaultKeyDirShort, "Folder where will be saved generated zone keys")
	flag.Bool("fs_keystore_enable", true, "Use filesystem key store (deprecated, ignored)")

	logging.SetLogLevel(logging.LogVerbose)

	err := cmd.Parse(defaultConfigPath, serviceName)
	if err != nil {
		log.WithError(err).WithField(logging.FieldKeyEventCode, logging.EventCodeErrorCantReadServiceConfig).
			Errorln("Can't parse args")
		os.Exit(1)
	}

	var keyStore keystore.StorageKeyCreation
	if filesystemV2.IsKeyDirectory(*outputDir) {
		keyStore = openKeyStoreV2(*outputDir)
	} else {
		keyStore = openKeyStoreV1(*outputDir)
	}

	id, publicKey, err := keyStore.GenerateZoneKey()
	if err != nil {
		log.WithError(err).Errorln("Can't add zone")
		os.Exit(1)
	}
	json, err := zone.ZoneDataToJSON(id, &keys.PublicKey{Value: publicKey})
	if err != nil {
		log.WithError(err).Errorln("Can't encode to json")
		os.Exit(1)
	}
	fmt.Println(string(json))
}

func openKeyStoreV1(output string) keystore.StorageKeyCreation {
	masterKey, err := keystore.GetMasterKeyFromEnvironment()
	if err != nil {
		log.WithError(err).Errorln("Cannot load master key")
		os.Exit(1)
	}
	scellEncryptor, err := keystore.NewSCellKeyEncryptor(masterKey)
	if err != nil {
		log.WithError(err).Errorln("Can't init scell encryptor")
		os.Exit(1)
	}
	keyStore, err := filesystem.NewFilesystemKeyStore(output, scellEncryptor)
	if err != nil {
		log.WithError(err).Errorln("Can't init key store")
		os.Exit(1)
	}
	return keyStore
}

func openKeyStoreV2(keyDirPath string) keystore.StorageKeyCreation {
	encryption, signature, err := keystoreV2.GetMasterKeysFromEnvironment()
	if err != nil {
		log.WithError(err).Errorln("Cannot load master key")
		os.Exit(1)
	}
	suite, err := keystoreV2.NewSCellSuite(encryption, signature)
	if err != nil {
		log.WithError(err).Error("failed to initialize Secure Cell crypto suite")
		os.Exit(1)
	}
	keyDir, err := filesystemV2.OpenDirectoryRW(keyDirPath, suite)
	if err != nil {
		log.WithError(err).WithField("path", keyDirPath).Error("cannot open key directory")
		os.Exit(1)
	}
	return keystoreV2.NewServerKeyStore(keyDir)
}
