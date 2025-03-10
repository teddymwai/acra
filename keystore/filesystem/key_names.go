/*
Copyright 2020, Cossack Labs Limited

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

package filesystem

import (
	"os"

	"github.com/cossacklabs/acra/keystore/v2/keystore/filesystem/backend/api"

	keystore2 "github.com/cossacklabs/acra/keystore"
)

func getSymmetricKeyName(id string) string {
	return id + `_sym`
}

func getTokenSymmetricKeyName(id []byte, ownerType keystore2.KeyOwnerType) string {
	var name string
	switch ownerType {
	case keystore2.KeyOwnerTypeClient:
		name = getClientIDSymmetricKeyName(id)
	case keystore2.KeyOwnerTypeZone:
		name = getZoneIDSymmetricKeyName(id)
	default:
		name = string(id)
	}
	return name + ".token"
}

func getClientIDSymmetricKeyName(id []byte) string {
	return getSymmetricKeyName(GetServerDecryptionKeyFilename(id))
}

func getZoneIDSymmetricKeyName(id []byte) string {
	return getSymmetricKeyName(GetZoneKeyFilename(id))
}

// IsKeyReadError return true if error is os.ErrNotExist compatible and NoKeyFoundExit
func IsKeyReadError(err error) bool {
	return (os.IsNotExist(err) || err == api.ErrNotExist) && keystore2.NoKeyFoundExit
}
