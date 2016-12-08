// Copyright 2016, Cossack Labs Limited
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package base

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/cossacklabs/acra/keystore"
	"github.com/cossacklabs/acra/utils"
	"github.com/cossacklabs/acra/zone"
	"github.com/cossacklabs/themis/gothemis/keys"
	"io"
	"log"
)

// error show that failed acra struct recognizing but data is may be valid
var FAKE_ACRA_STRUCT = errors.New("fake acra struct")

/*
which symbols can be used - 2 3 4 5 6 7
hex   char dec  bin
'22' - " - 34 - 0b100010
'33' - 3 - 51 - 0b110011
'44' - D - 68 - 0b1000100
'55' - U - 85 - 0b1010101
'66' - f - 102 - 0b1100110
'77' - w - 119 - 0b1110111
<"> decided as less possible occurance in sequence as 8 bytes in a row
*/

//var TAG_BEGIN = []byte{133, 32, 251}
const (
	TAG_SYMBOL byte = '"'
)

var TAG_BEGIN = []byte{TAG_SYMBOL, TAG_SYMBOL, TAG_SYMBOL, TAG_SYMBOL, TAG_SYMBOL, TAG_SYMBOL, TAG_SYMBOL, TAG_SYMBOL}

const (
	// length of EC public key
	PUBLIC_KEY_LENGTH = 45
	// length of 32 byte of symmetric key wrapped to smessage
	SMESSAGE_KEY_LENGTH = 84
	KEY_BLOCK_LENGTH    = PUBLIC_KEY_LENGTH + SMESSAGE_KEY_LENGTH

	SYMMETRIC_KEY_SIZE = 32
	DATA_LENGTH_SIZE   = 8
)

type DataDecryptor interface {
	// try match begin tag per byte
	MatchBeginTag(byte) bool
	// return true if all bytes from begin tag matched by MatchBeginTag
	IsMatched() bool
	// reset state of matching begin tag
	Reset()
	// return all matched begin tag bytes
	GetMatched() []byte
	// read, decode from db format block of data, decrypt symmetric key from
	// acrastruct using secure message
	// return decrypted data or data as is if fail
	// db specific
	ReadSymmetricKey(*keys.PrivateKey, io.Reader) ([]byte, []byte, error)
	// read and decrypt data or return as is if fail
	// db specific
	ReadData([]byte, []byte, io.Reader) ([]byte, error)
	GetTagBeginLength() int
}

type Decryptor interface {
	DataDecryptor
	// register key store that will be used for retrieving private keys
	SetKeyStore(keystore.KeyStore)
	// return private key for current connected client for decrypting symmetric
	// key with secure message
	GetPrivateKey() (*keys.PrivateKey, error)
	// register storage of callbacks for detected poison records
	SetPoisonCallbackStorage(*PoisonCallbackStorage)
	// get current storage of callbacks for detected poison records
	GetPoisonCallbackStorage() *PoisonCallbackStorage
	SetZoneMatcher(*zone.ZoneIdMatcher)
	SetPoisonKey([]byte)
	GetPoisonKey() []byte
	GetMatchedZoneId() []byte
	MatchZone(byte) bool
	IsWithZone() bool
	SetWithZone(bool)
	IsMatchedZone() bool
	ResetZoneMatch()
	IsWholeMatch() bool
	DecryptBlock([]byte) ([]byte, error)
	MatchZoneBlock([]byte)
	BeginTagIndex([]byte) (int, int)
}

func CheckReadWrite(n, expected_n int, err error, err_ch chan<- error) bool {
	if err != nil {
		err_ch <- err
		return false
	}
	if n != expected_n {
		err_ch <- errors.New(fmt.Sprintf("incorrect read/write count. %d != %d", n, expected_n))
		return false
	}
	return true
}

/* read per byte from reader and write to writer
until find TAG_BEGIN. if TAG_BEGIN found than try to read acra_struct and write to writer just decrypted data */
func DecryptStream(decryptor Decryptor, reader *bufio.Reader, writer *bufio.Writer, err_ch chan<- error) {
	char_buf := make([]byte, 1)
	inner_err_ch := make(chan error, 1)
	for {
		n, err := reader.Read(char_buf)
		if !CheckReadWrite(n, 1, err, err_ch) {
			/*TODO think how fix case when readed in stream matched begin tags and than EOF.
			TODO in this case these last bytes
			TODO but on other side we shouldn't reset in case when used recursively...
			*/
			//_, err = writer.Write(decryptor.GetMatched())
			//if !check_read_write(1, 1, err, err_ch) {
			//	return
			//}
			//decryptor.Reset()
			return
		}
		if decryptor.IsWithZone() {
			if !decryptor.IsMatchedZone() {
				decryptor.MatchZone(char_buf[0])
				n, err = writer.Write(char_buf)
				if !CheckReadWrite(n, 1, err, err_ch) {
					return
				}
				if reader.Buffered() == 0 {
					writer.Flush()
				}
				continue
			}
		}
		// here we have matched zone and loaded key if isWithZone
		if decryptor.MatchBeginTag(char_buf[0]) {
			if decryptor.IsMatched() {
				private_key, err := decryptor.GetPrivateKey()
				if err != nil {
					log.Printf("Warning: %v\n", utils.ErrorMessage("can't get zone key", err))
					err_ch <- err
					return
				}
				symmetric_key, raw_data, err := decryptor.ReadSymmetricKey(private_key, reader)
				if err != nil {
					if err == FAKE_ACRA_STRUCT {
						_, err = writer.Write(decryptor.GetMatched())
						if !CheckReadWrite(1, 1, err, err_ch) {
							return
						}
						decryptor.Reset()
						// process returned block from start
						DecryptStream(decryptor, bufio.NewReader(bytes.NewReader(raw_data)), writer, inner_err_ch)
						// should be any unexpected error or EOF
						err = <-inner_err_ch
						if err != io.EOF {
							err_ch <- err
							return
						}
						continue
					} else {
						log.Printf("Error: %v\n", utils.ErrorMessage("can't read symmetric key from acrastruct", err))
						err_ch <- err
						return
					}
				}
				if bytes.Equal(symmetric_key, decryptor.GetPoisonKey()) {
					log.Println("Warning: recognized poison record")
					err = decryptor.GetPoisonCallbackStorage().Call()
					if err != nil {
						log.Printf("Error: unexpected error in poison record callbacks - %v\n", err)
						err_ch <- err
						return
					}
					_, err = writer.Write(decryptor.GetMatched())
					if !CheckReadWrite(1, 1, err, err_ch) {
						return
					}
					decryptor.Reset()
					_, err = writer.Write(raw_data)
					if !CheckReadWrite(1, 1, err, err_ch) {
						return
					}
					continue
				}
				data, err := decryptor.ReadData(symmetric_key, decryptor.GetMatchedZoneId(), reader)
				if err != nil {
					if err == FAKE_ACRA_STRUCT {
						log.Println("Warning: can't decrypt data in acrastruct")
						// write begin tag
						_, err = writer.Write(decryptor.GetMatched())
						if !CheckReadWrite(1, 1, err, err_ch) {
							return
						}
						decryptor.ResetZoneMatch()
						decryptor.Reset()
						inner_data := append(raw_data, data...)
						// process returned block from start
						DecryptStream(decryptor, bufio.NewReader(bytes.NewReader(inner_data)), writer, inner_err_ch)
						inner_data = nil
						// should be any unexpected error or EOF
						err = <-inner_err_ch
						if err != io.EOF {
							err_ch <- err
							return
						}
						continue
					} else {
						err_ch <- err
						return
					}
				}
				n, err = writer.Write(data)
				if !CheckReadWrite(n, len(data), err, err_ch) {
					return
				}
				decryptor.ResetZoneMatch()
				decryptor.Reset()
				log.Println("Debug: decrypted acrastruct")
			}
		} else {
			// write buffered bytes after comparison
			_, err = writer.Write(decryptor.GetMatched())
			if !CheckReadWrite(1, 1, err, err_ch) {
				return
			}
			decryptor.Reset()

			n, err = writer.Write(char_buf)
			if !CheckReadWrite(n, 1, err, err_ch) {
				return
			}
		}

		if reader.Buffered() == 0 {
			writer.Flush()
		}
	}
}
