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

// Package filesystem implements keystores that write and reads keys from file system. Each keystore is responsible
// for generating keys for specific service, writing them to provided file path, reading and decrypting them.
// Server keystore generates AcraServer transport key pair and AcraStorage encryption keypair used for
// creating/decrypting AcraStructs.
// Connector keystore generates AcraConnector transport key pair.
// Translator keystore generates AcraTranslator transport key pair.
//
// https://github.com/cossacklabs/acra/wiki/Key-Management
package filesystem

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/cossacklabs/acra/cmd"
	"github.com/cossacklabs/acra/keystore"
	keystore2 "github.com/cossacklabs/acra/keystore"
	"github.com/cossacklabs/acra/keystore/lru"
	"github.com/cossacklabs/acra/utils"
	"github.com/cossacklabs/acra/zone"
	"github.com/cossacklabs/themis/gothemis/keys"
	log "github.com/sirupsen/logrus"
)

// PrivateFileMode used for all created files with private data
const PrivateFileMode = os.FileMode(0600)

// publicFileMode used for all created files with public data
const publicFileMode = os.FileMode(0644)

const keyDirMode = os.FileMode(0700)

// KeyStore represents keystore that reads keys from key folders, and stores them in memory.
type KeyStore struct {
	cache               keystore.Cache
	privateKeyDirectory string
	publicKeyDirectory  string
	fs                  Storage
	lock                *sync.RWMutex
	encryptor           keystore.KeyEncryptor
}

// NewFileSystemKeyStoreWithCacheSize represents keystore that reads keys from key folders, and stores them in cache.
func NewFileSystemKeyStoreWithCacheSize(directory string, encryptor keystore.KeyEncryptor, cacheSize int) (*KeyStore, error) {
	return NewCustomFilesystemKeyStore().KeyDirectory(directory).Encryptor(encryptor).CacheSize(cacheSize).Build()
}

// NewFilesystemKeyStore represents keystore that reads keys from key folders, and stores them in memory.
func NewFilesystemKeyStore(directory string, encryptor keystore.KeyEncryptor) (*KeyStore, error) {
	return NewCustomFilesystemKeyStore().KeyDirectory(directory).Encryptor(encryptor).Build()
}

// NewFilesystemKeyStoreTwoPath creates new KeyStore using separate folders for private and public keys.
func NewFilesystemKeyStoreTwoPath(privateKeyFolder, publicKeyFolder string, encryptor keystore.KeyEncryptor) (*KeyStore, error) {
	return NewCustomFilesystemKeyStore().KeyDirectories(privateKeyFolder, publicKeyFolder).Encryptor(encryptor).Build()
}

// KeyStoreBuilder allows to build a custom keystore.
type KeyStoreBuilder struct {
	privateKeyDir string
	publicKeyDir  string
	encryptor     keystore.KeyEncryptor
	storage       Storage
	cacheSize     int
}

// NewCustomFilesystemKeyStore allows a custom-made KeyStore to be built.
// You must set at least root key directories and provide a KeyEncryptor.
func NewCustomFilesystemKeyStore() *KeyStoreBuilder {
	return &KeyStoreBuilder{
		storage:   &DummyStorage{},
		cacheSize: keystore.InfiniteCacheSize,
	}
}

// KeyDirectory sets root key directory. Private and public keys will be kept together.
func (b *KeyStoreBuilder) KeyDirectory(directory string) *KeyStoreBuilder {
	b.privateKeyDir = directory
	b.publicKeyDir = directory
	return b
}

// KeyDirectories sets root key directories for private and public keys.
func (b *KeyStoreBuilder) KeyDirectories(privateKeyDir, publicKeyDir string) *KeyStoreBuilder {
	b.privateKeyDir = privateKeyDir
	b.publicKeyDir = publicKeyDir
	return b
}

// Encryptor sets cryptographic backend.
func (b *KeyStoreBuilder) Encryptor(encryptor keystore.KeyEncryptor) *KeyStoreBuilder {
	b.encryptor = encryptor
	return b
}

// Storage sets custom storage backend.
func (b *KeyStoreBuilder) Storage(storage Storage) *KeyStoreBuilder {
	b.storage = storage
	return b
}

// CacheSize sets cache size to use. By default cache size is unlimited,
func (b *KeyStoreBuilder) CacheSize(cacheSize int) *KeyStoreBuilder {
	b.cacheSize = cacheSize
	return b
}

var (
	errNoPrivateKeyDir = errors.New("private key directory not specified")
	errNoPublicKeyDir  = errors.New("public key directory not specified")
	errNoEncryptor     = errors.New("encryptor not specified")
)

// Build constructs a KeyStore with specified parameters.
func (b *KeyStoreBuilder) Build() (*KeyStore, error) {
	if b.privateKeyDir == "" {
		return nil, errNoPrivateKeyDir
	}
	if b.publicKeyDir == "" {
		return nil, errNoPublicKeyDir
	}
	if b.encryptor == nil {
		return nil, errNoEncryptor
	}
	return newFilesystemKeyStore(b.privateKeyDir, b.publicKeyDir, b.storage, b.encryptor, b.cacheSize)
}

// IsKeyDirectory checks if the local directory contains a keystore v1.
// This is a conservative check.
// That is, positive return value does not mean that the directory contains *a valid* keystore.
// However, false value means that the directory is definitely not a valid keystore.
// In particular, false is returned if the directory does not exists or cannot be opened.
func IsKeyDirectory(keyDirectory string) bool {
	storage, err := openKeyStorage()
	if err != nil {
		log.WithError(err).Debug("Failed to open key storage for version check")
		return false
	}
	fi, err := storage.Stat(keyDirectory)
	if err != nil {
		log.WithError(err).WithField("path", keyDirectory).Debug("Failed to stat key directory for version check")
		return false
	}
	if !fi.IsDir() {
		log.WithField("path", keyDirectory).Debug("Key directory is not a directory")
		return false
	}
	files, err := storage.ReadDir(keyDirectory)
	if err != nil {
		log.WithError(err).WithField("path", keyDirectory).Debug("Failed to read key directory for version check")
		return false
	}
	if len(files) == 0 {
		log.WithField("path", keyDirectory).Debug("Key directory is empty")
		return false
	}
	return true
}

func openKeyStorage() (Storage, error) {
	redis := cmd.GetRedisParameters()
	if redis.KeysConfigured() {
		return NewRedisStorage(redis.HostPort, redis.Password, redis.DBKeys, nil)
	}
	return &DummyStorage{}, nil
}

func newFilesystemKeyStore(privateKeyFolder, publicKeyFolder string, storage Storage, encryptor keystore.KeyEncryptor, cacheSize int) (*KeyStore, error) {
	fi, err := storage.Stat(privateKeyFolder)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if !os.IsNotExist(err) {
		const expectedPermission = "-rwx------"
		if runtime.GOOS == "linux" && fi.Mode().Perm().String() != expectedPermission {
			log.Errorf("Keystore folder has an incorrect permissions %s, expected: %s", fi.Mode().Perm().String(), expectedPermission)
			return nil, errors.New("keystore folder has an incorrect permissions")
		}
	}
	_, err = storage.Stat(publicKeyFolder)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var cache keystore.Cache
	if cacheSize == keystore.WithoutCache {
		cache = keystore.NoCache{}
	} else {
		cache, err = lru.NewCacheKeystoreWrapper(cacheSize)
		if err != nil {
			return nil, err
		}
	}
	store := &KeyStore{privateKeyDirectory: privateKeyFolder, publicKeyDirectory: publicKeyFolder,
		cache: cache, lock: &sync.RWMutex{}, encryptor: encryptor, fs: storage}
	// set callback on cache value removing

	return store, nil
}

func (store *KeyStore) generateKeyPair(filename string, clientID []byte) (*keys.Keypair, error) {
	keypair, err := keys.New(keys.TypeEC)
	if err != nil {
		return nil, err
	}
	if err := store.SaveKeyPairWithFilename(keypair, filename, clientID); err != nil {
		return nil, err
	}
	return keypair, nil
}

// SaveKeyPairWithFilename save encrypted private key and public key to configured folders
func (store *KeyStore) SaveKeyPairWithFilename(keypair *keys.Keypair, filename string, id []byte) error {
	privateKeysFolder := filepath.Dir(store.GetPrivateKeyFilePath(filename))
	err := store.fs.MkdirAll(privateKeysFolder, keyDirMode)
	if err != nil {
		return err
	}

	publicKeysFolder := filepath.Dir(store.GetPublicKeyFilePath(filename))
	err = store.fs.MkdirAll(publicKeysFolder, keyDirMode)
	if err != nil {
		return err
	}

	encryptedPrivate, err := store.encryptor.Encrypt(keypair.Private.Value, id)
	if err != nil {
		return err
	}
	err = store.WritePrivateKey(store.GetPrivateKeyFilePath(filename), encryptedPrivate)
	if err != nil {
		return err
	}
	err = store.WritePublicKey(store.GetPublicKeyFilePath(fmt.Sprintf("%s.pub", filename)), keypair.Public.Value)
	if err != nil {
		return err
	}
	store.cache.Add(filename, encryptedPrivate)
	return nil
}

func (store *KeyStore) generateKey(filename string, length uint8) ([]byte, error) {
	randomBytes := make([]byte, length)
	_, err := rand.Read(randomBytes)
	// Note that err == nil only if we read len(b) bytes.
	if err != nil {
		log.Error(err)
		return nil, err
	}
	dirpath := filepath.Dir(store.GetPrivateKeyFilePath(filename))
	err = store.fs.MkdirAll(dirpath, keyDirMode)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	err = store.WritePrivateKey(store.GetPrivateKeyFilePath(filename), randomBytes)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return randomBytes, nil
}

// WritePrivateKey writes private key from data to filename
func (store *KeyStore) WritePrivateKey(filename string, data []byte) error {
	return store.WriteKeyFile(filename, data, PrivateFileMode)
}

// WritePublicKey writes public key from data to filename
func (store *KeyStore) WritePublicKey(filename string, data []byte) error {
	return store.WriteKeyFile(filename, data, publicFileMode)
}

// ReadKeyFile reads raw key data for given filename.
func (store *KeyStore) ReadKeyFile(filename string) ([]byte, error) {
	return store.fs.ReadFile(filename)
}

// WriteKeyFile updates key data, creating a new file if necessary.
func (store *KeyStore) WriteKeyFile(filename string, data []byte, mode os.FileMode) error {
	// We do quite a few filesystem manipulations to maintain old key data. Ensure that
	// no data is lost due to errors or power faults. "filename" must contain either
	// new key data on success, or old key data on error.
	tmpFilename, err := store.fs.TempFile(filename, mode)
	if err != nil {
		return err
	}
	err = store.fs.WriteFile(tmpFilename, data, mode)
	if err != nil {
		return err
	}
	err = store.backupHistoricalKeyFile(filename)
	if err != nil {
		return err
	}
	err = store.fs.Rename(tmpFilename, filename)
	if err != nil {
		return err
	}
	return nil
}

func (store *KeyStore) backupHistoricalKeyFile(filename string) error {
	// If the file does not exist then there's nothing to backup
	_, err := store.fs.Stat(filename)
	if os.IsNotExist(err) {
		return nil
	}
	err = store.fs.MkdirAll(getHistoryDirName(filename), keyDirMode)
	if err != nil {
		return err
	}
	backupName := getNewHistoricalFileName(filename)
	// Try making a hard link if possible to avoid actually copying file content
	err = store.fs.Link(filename, backupName)
	if err == nil {
		return nil
	}
	return store.fs.Copy(filename, backupName)
}

// generateZoneKey for specific zone id. Will be generated new key pair and private key will be overwrited
func (store *KeyStore) generateZoneKey(id []byte) ([]byte, []byte, error) {
	/* save private key in fs, return id and public key*/
	keypair, err := store.generateKeyPair(GetZoneKeyFilename(id), id)
	if err != nil {
		return []byte{}, []byte{}, err
	}
	store.lock.Lock()
	defer store.lock.Unlock()
	encryptedKey, err := store.encryptor.Encrypt(keypair.Private.Value, id)
	if err != nil {
		return nil, nil, nil
	}
	utils.ZeroizePrivateKey(keypair.Private)
	// cache key
	store.cache.Add(GetZoneKeyFilename(id), encryptedKey)
	return id, keypair.Public.Value, nil
}

// GenerateZoneKey generates zone ID and zone key pair, encrypts private key using zoneID as context,
// and saves encrypted PK in the filem returns zoneID and public key.
// Returns error if generation or encryption fail.
func (store *KeyStore) GenerateZoneKey() ([]byte, []byte, error) {
	var id []byte
	for {
		// generate until key not exists
		id = zone.GenerateZoneID()
		if !store.HasZonePrivateKey(id) {
			break
		}
	}
	return store.generateZoneKey(id)
}

// GetPrivateKeyFilePath return path for file with private key with configured folder for store
func (store *KeyStore) GetPrivateKeyFilePath(filename string) string {
	return fmt.Sprintf("%s%s%s", store.privateKeyDirectory, string(os.PathSeparator), filename)
}

// GetPublicKeyFilePath return path for file with public key with configured folder for store
func (store *KeyStore) GetPublicKeyFilePath(filename string) string {
	return fmt.Sprintf("%s%s%s", store.publicKeyDirectory, string(os.PathSeparator), filename)
}

// GetHistoricalPrivateKeyFilenames return filenames for current and rotated keys
func (store *KeyStore) GetHistoricalPrivateKeyFilenames(filename string) ([]string, error) {
	// getHistoricalFilePaths() expects a path, not a name, but we must return names.
	// Add private key directory path and then remove it to avoid directory switching.
	fullPath := filepath.Join(store.privateKeyDirectory, filename)
	paths, err := getHistoricalFilePaths(fullPath, store.fs)
	if err != nil {
		return nil, err
	}
	for i, path := range paths {
		p, err := filepath.Rel(store.privateKeyDirectory, path)
		if err != nil {
			return nil, err
		}
		paths[i] = p
	}
	return paths, nil
}

func (store *KeyStore) loadPrivateKey(path string) (*keys.PrivateKey, error) {
	fi, err := store.fs.Stat(path)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == "linux" && fi.Mode().Perm() > PrivateFileMode {
		log.Errorf("Private key file %v has incorrect permissions %s, expected: %s", path, fi.Mode().Perm().String(), PrivateFileMode.String())
		return nil, fmt.Errorf("private key file %v has incorrect permissions", path)
	}
	// Strictly speaking, this is racy because the file we were statting
	// may not be the same as we will be reading, but it's okay in this case.
	key, err := store.fs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &keys.PrivateKey{Value: key}, nil
}

func (store *KeyStore) loadPublicKey(path string) (*keys.PublicKey, error) {
	key, err := store.fs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &keys.PublicKey{Value: key}, nil
}

func (store *KeyStore) getPrivateKeyByFilename(id []byte, filename string) (*keys.PrivateKey, error) {
	store.lock.Lock()
	defer store.lock.Unlock()
	encryptedKey, ok := store.cache.Get(filename)
	if !ok {
		encryptedPrivateKey, err := store.loadPrivateKey(store.GetPrivateKeyFilePath(filename))
		if err != nil {
			return nil, err
		}
		encryptedKey = encryptedPrivateKey.Value
	}

	decryptedKey, err := store.encryptor.Decrypt(encryptedKey, id)
	if err != nil {
		return nil, err
	}
	log.Debugf("Load key from fs: %s", filename)
	store.cache.Add(filename, encryptedKey)
	return &keys.PrivateKey{Value: decryptedKey}, nil
}

func (store *KeyStore) getPrivateKeysByFilenames(id []byte, filenames []string) ([]*keys.PrivateKey, error) {
	// TODO: this can be optimized to avoid thrashing store.lock and repeatedly revalidating id
	// by copy-pasting getPrivateKeyByFilename() and extending that to retrieve multiple keys
	privateKeys := make([]*keys.PrivateKey, len(filenames))
	for i, name := range filenames {
		key, err := store.getPrivateKeyByFilename(id, name)
		if err != nil {
			utils.ZeroizePrivateKeys(privateKeys)
			return nil, err
		}
		privateKeys[i] = key
	}
	return privateKeys, nil
}

// getPublicKeyByFilename return public key from cache or load from filesystem, store in cache and return
func (store *KeyStore) getPublicKeyByFilename(filename string) (*keys.PublicKey, error) {
	binKey, ok := store.cache.Get(filename)
	if !ok {
		publicKey, err := store.loadPublicKey(filename)
		if err != nil {
			return nil, err
		}
		store.cache.Add(filename, publicKey.Value)
		return publicKey, nil
	}
	return &keys.PublicKey{Value: binKey}, nil
}

// GetZonePublicKey return PublicKey by zoneID from cache or load from main store
func (store *KeyStore) GetZonePublicKey(zoneID []byte) (*keys.PublicKey, error) {
	fname := store.GetPublicKeyFilePath(getZonePublicKeyFilename(zoneID))
	return store.getPublicKeyByFilename(fname)
}

// GetClientIDEncryptionPublicKey return PublicKey by clientID from cache or load from main store
func (store *KeyStore) GetClientIDEncryptionPublicKey(clientID []byte) (*keys.PublicKey, error) {
	fname := store.GetPublicKeyFilePath(
		// use correct suffix for public keys
		getPublicKeyFilename(
			// use correct suffix as type of key
			[]byte(GetServerDecryptionKeyFilename(clientID))))
	return store.getPublicKeyByFilename(fname)
}

// GetZonePrivateKey reads encrypted zone private key from fs, decrypts it with master key and zoneId
// and returns plaintext private key, or reading/decryption error.
func (store *KeyStore) GetZonePrivateKey(id []byte) (*keys.PrivateKey, error) {
	fname := GetZoneKeyFilename(id)
	return store.getPrivateKeyByFilename(id, fname)
}

// HasZonePrivateKey returns if private key for this zoneID exists in cache or is written to fs.
func (store *KeyStore) HasZonePrivateKey(id []byte) bool {
	if !keystore.ValidateID(id) {
		return false
	}
	// add caching false answers. now if key doesn't exists than always checks on fs
	// it's system call and slow.
	if len(id) == 0 {
		return false
	}
	fname := GetZoneKeyFilename(id)
	store.lock.RLock()
	defer store.lock.RUnlock()
	_, ok := store.cache.Get(fname)
	if ok {
		return true
	}
	exists, _ := store.fs.Exists(store.GetPrivateKeyFilePath(fname))
	return exists
}

// GetZonePrivateKeys reads all historical encrypted zone private keys from fs,
// decrypts them with master key and zoneId, and returns plaintext private keys,
// or reading/decryption error.
func (store *KeyStore) GetZonePrivateKeys(id []byte) ([]*keys.PrivateKey, error) {
	filenames, err := store.GetHistoricalPrivateKeyFilenames(GetZoneKeyFilename(id))
	if err != nil {
		return nil, err
	}
	return store.getPrivateKeysByFilenames(id, filenames)
}

// GetPeerPublicKey returns public key for this clientID, gets it from cache or reads from fs.
func (store *KeyStore) GetPeerPublicKey(id []byte) (*keys.PublicKey, error) {
	if !keystore.ValidateID(id) {
		return nil, keystore.ErrInvalidClientID
	}
	fname := getPublicKeyFilename(id)
	store.lock.Lock()
	defer store.lock.Unlock()
	key, ok := store.cache.Get(fname)
	if ok {
		log.Debugf("Load cached key: %s", fname)
		return &keys.PublicKey{Value: key}, nil
	}
	publicKey, err := store.loadPublicKey(store.GetPublicKeyFilePath(fname))
	if err != nil {
		return nil, err
	}
	log.Debugf("Load key from fs: %s", fname)
	store.cache.Add(fname, publicKey.Value)
	return publicKey, nil
}

// GetPrivateKey reads encrypted client private key from fs, decrypts it with master key and clientID,
// and returns plaintext private key, or reading/decryption error.
func (store *KeyStore) GetPrivateKey(id []byte) (*keys.PrivateKey, error) {
	fname := getServerKeyFilename(id)
	return store.getPrivateKeyByFilename(id, fname)
}

// GetServerDecryptionPrivateKey reads encrypted server storage private key from fs,
// decrypts it with master key and clientID,
// and returns plaintext private key, or reading/decryption error.
func (store *KeyStore) GetServerDecryptionPrivateKey(id []byte) (*keys.PrivateKey, error) {
	fname := GetServerDecryptionKeyFilename(id)
	return store.getPrivateKeyByFilename(id, fname)
}

// GetServerDecryptionPrivateKeys reads encrypted server storage private keys from fs,
// decrypts them with master key and clientID, and returns plaintext private keys,
// or reading/decryption error.
func (store *KeyStore) GetServerDecryptionPrivateKeys(id []byte) ([]*keys.PrivateKey, error) {
	filenames, err := store.GetHistoricalPrivateKeyFilenames(GetServerDecryptionKeyFilename(id))
	if err != nil {
		return nil, err
	}
	return store.getPrivateKeysByFilenames(id, filenames)
}

// GenerateConnectorKeys generates AcraConnector transport EC keypair using clientID as part of key name.
// Writes encrypted private key and plaintext public key to fs.
// Returns error if writing/encryption failed.
func (store *KeyStore) GenerateConnectorKeys(id []byte) error {
	if !keystore.ValidateID(id) {
		return keystore.ErrInvalidClientID
	}
	filename := getConnectorKeyFilename(id)

	_, err := store.generateKeyPair(filename, id)
	if err != nil {
		return err
	}
	return nil
}

// GenerateServerKeys generates AcraServer transport EC keypair using clientID as part of key name.
// Writes encrypted private key and plaintext public key to fs.
// Returns error if writing/encryption failed.
func (store *KeyStore) GenerateServerKeys(id []byte) error {
	if !keystore.ValidateID(id) {
		return keystore.ErrInvalidClientID
	}
	filename := getServerKeyFilename(id)
	_, err := store.generateKeyPair(filename, id)
	if err != nil {
		return err
	}
	return nil
}

// GenerateTranslatorKeys generates AcraTranslator transport EC keypair using clientID as part of key name.
// Writes encrypted private key and plaintext public key to fs.
// Returns error if writing/encryption failed.
func (store *KeyStore) GenerateTranslatorKeys(id []byte) error {
	if !keystore.ValidateID(id) {
		return keystore.ErrInvalidClientID
	}
	filename := getTranslatorKeyFilename(id)
	_, err := store.generateKeyPair(filename, id)
	if err != nil {
		return err
	}
	return nil
}

// GenerateDataEncryptionKeys generates Storage EC keypair for encrypting/decrypting data
// using clientID as part of key name.
// Writes encrypted private key and plaintext public key to fs.
// Returns error if writing/encryption failed.
func (store *KeyStore) GenerateDataEncryptionKeys(id []byte) error {
	if !keystore.ValidateID(id) {
		return keystore.ErrInvalidClientID
	}
	_, err := store.generateKeyPair(GetServerDecryptionKeyFilename(id), id)
	if err != nil {
		return err
	}
	return nil
}

// ListKeys enumerates keys present in the keystore.
func (store *KeyStore) ListKeys() ([]keystore.KeyDescription, error) {
	// In Acra CE this method is implemented only for keystore v2.
	return nil, keystore.ErrNotImplemented
}

// Reset clears all cached keys
func (store *KeyStore) Reset() {
	store.cache.Clear()
}

// GetPoisonKeyPair generates EC keypair for encrypting/decrypting poison records, and writes it to fs
// encrypting private key or reads existing keypair from fs.
// Returns keypair or error if generation/decryption failed.
func (store *KeyStore) GetPoisonKeyPair() (*keys.Keypair, error) {
	privatePath := store.GetPrivateKeyFilePath(PoisonKeyFilename)
	publicPath := store.GetPublicKeyFilePath(poisonKeyFilenamePublic)
	privateExists, err := store.fs.Exists(privatePath)
	if err != nil {
		return nil, err
	}
	publicExists, err := store.fs.Exists(publicPath)
	if err != nil {
		return nil, err
	}
	if privateExists && publicExists {
		private, err := store.loadPrivateKey(privatePath)
		if err != nil {
			return nil, err
		}
		if private.Value, err = store.encryptor.Decrypt(private.Value, []byte(PoisonKeyFilename)); err != nil {
			return nil, err
		}
		public, err := store.loadPublicKey(publicPath)
		if err != nil {
			return nil, err
		}
		return &keys.Keypair{Public: public, Private: private}, nil
	}
	log.Debug("Generate poison key pair")
	return store.generateKeyPair(PoisonKeyFilename, []byte(PoisonKeyFilename))
}

// GetPoisonPrivateKeys returns all private keys used to decrypt poison records, from newest to oldest.
// If a poison record does not exist, it is created and its sole private key is returned.
// Returns a list of private poison keys (possibly empty), or an error if decryption fails.
func (store *KeyStore) GetPoisonPrivateKeys() ([]*keys.PrivateKey, error) {
	poisonKeyExists, err := store.fs.Exists(store.GetPrivateKeyFilePath(PoisonKeyFilename))
	if err != nil {
		return nil, err
	}
	// If there is no poison record keypair, generated one and returns its private key.
	if !poisonKeyExists {
		log.Debug("Generate poison key pair")
		keypair, err := store.generateKeyPair(PoisonKeyFilename, []byte(PoisonKeyFilename))
		if err != nil {
			return nil, err
		}
		return []*keys.PrivateKey{keypair.Private}, nil
	}
	// If some poison keypairs exist, pull their private keys.
	filenames, err := store.GetHistoricalPrivateKeyFilenames(PoisonKeyFilename)
	if err != nil {
		return nil, err
	}
	return store.getPrivateKeysByFilenames([]byte(PoisonKeyFilename), filenames)
}

// GetPoisonSymmetricKeys returns all symmetric keys used to decrypt poison records with AcraBlock, from newest to oldest.
// If a poison record does not exist, it is created and its sole symmetric key is returned.
// Returns a list of symmetric poison keys (possibly empty), or an error if decryption fails.
func (store *KeyStore) GetPoisonSymmetricKeys() ([][]byte, error) {
	keyFileName := getSymmetricKeyName(PoisonKeyFilename)
	poisonKeyExists, err := store.fs.Exists(store.GetPrivateKeyFilePath(keyFileName))
	if err != nil {
		log.Debug("Can't check poison key existence")
		return nil, err
	}
	log.WithError(err).WithField("exists", poisonKeyExists).Debug("Get poison sym keys")
	// If there is no poison record keypair, generated one and returns its private key.
	if !poisonKeyExists {
		log.Debugln("Generate poison symmetric key")

		if err := store.fs.MkdirAll(filepath.Dir(store.GetPrivateKeyFilePath(keyFileName)), keyDirMode); err != nil {
			log.Debug("Can't generate .poison_key directory")
			return nil, err
		}

		err := store.generateAndSaveSymmetricKey([]byte(keyFileName), store.GetPrivateKeyFilePath(keyFileName))
		if err != nil {
			log.Debug("Can't generate new poison sym key")
			return nil, err
		}
	}
	return store.getSymmetricKeys([]byte(keyFileName), keyFileName)
}

// RotateZoneKey generate new key pair for ZoneId, overwrite private key with new and return new public key
func (store *KeyStore) RotateZoneKey(zoneID []byte) ([]byte, error) {
	_, public, err := store.generateZoneKey(zoneID)
	return public, err
}

// RotateSymmetricZoneKey generate new symmetric key for ZoneId, overwrite private key with new
func (store *KeyStore) RotateSymmetricZoneKey(zoneID []byte) error {
	keyName := getZoneIDSymmetricKeyName(zoneID)
	return store.generateAndSaveSymmetricKey(zoneID, store.GetPrivateKeyFilePath(keyName))
}

// SaveZoneKeypair save or overwrite zone keypair
func (store *KeyStore) SaveZoneKeypair(id []byte, keypair *keys.Keypair) error {
	filename := GetZoneKeyFilename(id)
	return store.SaveKeyPairWithFilename(keypair, filename, id)
}

// SaveConnectorKeypair save or overwrite acra-connector keypair
func (store *KeyStore) SaveConnectorKeypair(id []byte, keypair *keys.Keypair) error {
	filename := getConnectorKeyFilename(id)
	return store.SaveKeyPairWithFilename(keypair, filename, id)
}

// SaveServerKeypair save or overwrite acra-server keypair
func (store *KeyStore) SaveServerKeypair(id []byte, keypair *keys.Keypair) error {
	filename := getServerKeyFilename(id)
	return store.SaveKeyPairWithFilename(keypair, filename, id)
}

// SaveTranslatorKeypair save or overwrite acra-translator keypair
func (store *KeyStore) SaveTranslatorKeypair(id []byte, keypair *keys.Keypair) error {
	filename := getTranslatorKeyFilename(id)
	return store.SaveKeyPairWithFilename(keypair, filename, id)
}

// SaveDataEncryptionKeys save or overwrite decryption keypair for client id
func (store *KeyStore) SaveDataEncryptionKeys(id []byte, keypair *keys.Keypair) error {
	filename := GetServerDecryptionKeyFilename(id)
	return store.SaveKeyPairWithFilename(keypair, filename, id)
}

// destroyKeyWithFilename removes private and public key with given filename.
func (store *KeyStore) destroyKeyWithFilename(filename string) error {
	// Purge private key data from cache too.
	store.cache.Add(filename, nil)

	// Remove key files. It's okay if they are already removed (or never existed).
	// Keystore v1 does not differentiate between 'destroying' and 'removing' keys
	// because multiple functinons depend on the key file to be absent, not empty.
	err := store.fs.Remove(store.GetPrivateKeyFilePath(filename))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	err = store.fs.Remove(store.GetPublicKeyFilePath(filename + ".pub"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// DestroyConnectorKeypair destroys currently used AcraConnector transport keypair for given clientID.
func (store *KeyStore) DestroyConnectorKeypair(id []byte) error {
	filename := getConnectorKeyFilename(id)
	return store.destroyKeyWithFilename(filename)
}

// DestroyServerKeypair destroys currently used AcraServer transport keypair for given clientID.
func (store *KeyStore) DestroyServerKeypair(id []byte) error {
	filename := getServerKeyFilename(id)
	return store.destroyKeyWithFilename(filename)
}

// DestroyTranslatorKeypair destroys currently used AcraTranslator transport keypair for given clientID.
func (store *KeyStore) DestroyTranslatorKeypair(id []byte) error {
	filename := getTranslatorKeyFilename(id)
	return store.destroyKeyWithFilename(filename)
}

// Add value to inner cache
func (store *KeyStore) Add(keyID string, keyValue []byte) {
	store.cache.Add(keyID, keyValue)
}

// Get value from inner cache
func (store *KeyStore) Get(keyID string) ([]byte, bool) {
	return store.cache.Get(keyID)
}

// GetHMACSecretKey return key for hmac calculation according to id
func (store *KeyStore) GetHMACSecretKey(id []byte) ([]byte, error) {
	filename := getHmacKeyFilename(id)
	var err error
	encryptedKey, ok := store.Get(filename)
	if !ok {
		encryptedKey, err = store.ReadKeyFile(store.GetPrivateKeyFilePath(filename))
		if err != nil {
			return nil, err
		}
	}

	decryptedKey, err := store.encryptor.Decrypt(encryptedKey, id)
	if err != nil {
		return nil, err
	}
	log.Debugf("Load key from fs: %s", filename)
	store.Add(filename, encryptedKey)
	return decryptedKey, nil
}

// GenerateHmacKey key for hmac calculation in in folder for private keys
func (store *KeyStore) GenerateHmacKey(id []byte) error {
	log.Debugln("Generate HMAC")
	key, err := keystore.GenerateSymmetricKey()
	if err != nil {
		return err
	}
	encryptedKey, err := store.encryptor.Encrypt(key, id)
	if err != nil {
		return err
	}
	utils.ZeroizeSymmetricKey(key)

	path := store.GetPrivateKeyFilePath(getHmacKeyFilename(id))
	err = store.WriteKeyFile(path, encryptedKey, PrivateFileMode)
	if err != nil {
		return err
	}

	store.Add(getHmacKeyFilename(id), encryptedKey)

	return nil
}

// GenerateLogKey key for log integrity check calculation in folder for private keys
func (store *KeyStore) GenerateLogKey() error {
	log.Debugln("Generate secure log key")
	key, err := keystore.GenerateSymmetricKey()
	if err != nil {
		return err
	}
	encryptedKey, err := store.encryptor.Encrypt(key, []byte(SecureLogKeyFilename))
	if err != nil {
		return err
	}
	utils.ZeroizeSymmetricKey(key)

	path := store.GetPrivateKeyFilePath(getLogKeyFilename())
	err = store.WriteKeyFile(path, encryptedKey, PrivateFileMode)
	if err != nil {
		return err
	}

	store.Add(getLogKeyFilename(), encryptedKey)

	return nil
}

// GetLogSecretKey return key for log integrity checks
func (store *KeyStore) GetLogSecretKey() ([]byte, error) {
	filename := getLogKeyFilename()
	var err error
	encryptedKey, ok := store.Get(filename)
	if !ok {
		encryptedKey, err = store.ReadKeyFile(store.GetPrivateKeyFilePath(filename))
		if err != nil {
			return nil, err
		}
	}
	decryptedKey, err := store.encryptor.Decrypt(encryptedKey, []byte(SecureLogKeyFilename))
	if err != nil {
		return nil, err
	}
	log.Debugf("Load key from fs: %s", filename)
	store.Add(filename, encryptedKey)
	return decryptedKey, nil
}

// generateSymmetricKey generate symmetric key with specific identifier
func (store *KeyStore) generateAndSaveSymmetricKey(id []byte, filename string) error {
	symKey, err := keystore.GenerateSymmetricKey()
	if err != nil {
		return err
	}
	encryptedSymKey, err := store.encryptor.Encrypt(symKey, id)
	if err != nil {
		return err
	}
	return store.WritePrivateKey(filename, encryptedSymKey)
}

// GetSymmetricKey return symmetric key with specific identifier
func (store *KeyStore) readEncryptedKey(id []byte, filename string) ([]byte, error) {
	encryptedSymKey, err := store.ReadKeyFile(filename)
	if err != nil {
		return nil, err
	}
	return store.encryptor.Decrypt(encryptedSymKey, id)
}

// GenerateClientIDSymmetricKey generate symmetric key for specified client id
func (store *KeyStore) GenerateClientIDSymmetricKey(id []byte) error {
	keyName := getClientIDSymmetricKeyName(id)
	return store.generateAndSaveSymmetricKey(id, store.GetPrivateKeyFilePath(keyName))
}

// GenerateZoneIDSymmetricKey generate symmetric key for specified zone id
func (store *KeyStore) GenerateZoneIDSymmetricKey(id []byte) error {
	keyName := getZoneIDSymmetricKeyName(id)
	return store.generateAndSaveSymmetricKey(id, store.GetPrivateKeyFilePath(keyName))
}

// GeneratePoisonRecordSymmetricKey generate symmetric key for poison records
func (store *KeyStore) GeneratePoisonRecordSymmetricKey() error {
	keyName := getSymmetricKeyName(PoisonKeyFilename)
	exists, err := store.fs.Exists(keyName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if err := store.fs.MkdirAll(filepath.Dir(store.GetPrivateKeyFilePath(keyName)), keyDirMode); err != nil {
		return err
	}
	return store.generateAndSaveSymmetricKey([]byte(keyName), store.GetPrivateKeyFilePath(keyName))
}

func (store *KeyStore) getSymmetricKeys(id []byte, keyname string) ([][]byte, error) {
	keys := make([][]byte, 0, 1)
	historicalKeys, err := store.GetHistoricalPrivateKeyFilenames(keyname)
	if err != nil {
		log.Debug("Can't get historical private key filenames")
		for _, key := range keys {
			utils.ZeroizeSymmetricKey(key)
		}
		return nil, err
	}
	for _, path := range historicalKeys {
		key, err := store.readEncryptedKey(id, store.GetPrivateKeyFilePath(path))
		if err != nil {
			for _, key := range keys {
				utils.ZeroizeSymmetricKey(key)
			}
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// GetClientIDSymmetricKeys return symmetric key for specified client id
func (store *KeyStore) GetClientIDSymmetricKeys(id []byte) ([][]byte, error) {
	keyName := getClientIDSymmetricKeyName(id)
	return store.getSymmetricKeys(id, keyName)
}

// GetZoneIDSymmetricKeys return symmetric key for specified zone id
func (store *KeyStore) GetZoneIDSymmetricKeys(id []byte) ([][]byte, error) {
	keyName := getZoneIDSymmetricKeyName(id)
	return store.getSymmetricKeys(id, keyName)
}

// GetDecryptionTokenSymmetricKeys return symmetric keys which may be used to decrypt encrypted token
func (store *KeyStore) GetDecryptionTokenSymmetricKeys(id []byte, ownerType keystore2.KeyOwnerType) ([][]byte, error) {
	keyName := getTokenSymmetricKeyName(id, ownerType)
	return store.getSymmetricKeys(id, keyName)
}

// GetEncryptionTokenSymmetricKey return symmetric key which should be used to encrypt tokens
func (store *KeyStore) GetEncryptionTokenSymmetricKey(id []byte, ownerType keystore2.KeyOwnerType) ([]byte, error) {
	keyName := getTokenSymmetricKeyName(id, ownerType)
	keys, err := store.getSymmetricKeys(id, keyName)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, keystore2.ErrKeysNotFound
	}
	return keys[0], nil
}

// GenerateTokenSymmetricKey new symmetric key in keystore
func (store *KeyStore) GenerateTokenSymmetricKey(id []byte, ownerType keystore2.KeyOwnerType) error {
	keyName := getTokenSymmetricKeyName(id, ownerType)
	return store.generateAndSaveSymmetricKey(id, store.GetPrivateKeyFilePath(keyName))
}
