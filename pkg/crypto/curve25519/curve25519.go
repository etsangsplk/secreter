// Copyright 2019 The Secreter Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package curve25519 implements the unauthenticated public-key encryption scheme.
//
// Only the recipient can decrypt the message using the corresponding private key.
// While the recipient can verify the integrity of the message, it cannot verify
// the identity of the sender.
//
// Sender anonymously encrypts the message using the recipient's public key (PK)
// and the ephemeral key-pair (EPK, ESK) whose secret part is used only for
// computing the shared key and only once per encryption. Ephemeral secret key
// is neither stored nor transmitted anywhere and is destroyed after the
// encryption has finished.
//
// XChaCha20-Poly1305 is used for AEAD.
// Additional data (AD) for the AEAD is composed via concatenating PK and EPK.
// Shared key computed using Curve25519 is used as the initial key material for HKDF.
// Additional data is used as salt. Key and nonce for XChaCha20-Poly1305 are generated via HKDF.
//
// Additional data is concatenated with the resulting ciphertext. Public key is
// stored alongside the encrypted message to simplify the search of the
// corresponding private key performed by the recipient.
//
// Ephemeral key pair is generated every time the encryption is called.
// Thus, a new key pair will be generated for the same plaintext every time the
// encryption is called.
//
// Generating ephemeral keys for every encrypting operation guarantees uniqueness
// of derived keys and nonce.
//
// Resulting message structure:
//   [Header:1||EPK:32||ciphertext]
//   EPK - Ephemeral public key
package curve25519

import (
	"crypto/rand"
	"errors"
	"io"

	"github.com/amaizfinance/secreter/pkg/crypto"
	"github.com/amaizfinance/secreter/pkg/crypto/xchacha20poly1305"

	"golang.org/x/crypto/curve25519"
)

const (
	// KeySize is the size, in bytes, of public and private keys
	KeySize = 32

	headerOffset             = crypto.HeaderSize
	ephemeralPublicKeyOffset = headerOffset + KeySize
	cipherTextMinSize        = ephemeralPublicKeyOffset + xchacha20poly1305.CiphertextMinSize
)

var (
	errKeySize        = errors.New("curve25519: bad key length")
	errCipherTextSize = errors.New("curve25519: ciphertext too short")
)

type encryptDecrypter struct {
	publicKey, privateKey []byte
	rand                  io.Reader
}

// Encrypt will encrypt the message using an ephemeral key pair
func (e encryptDecrypter) Encrypt(plaintext []byte) ([]byte, error) {
	if len(e.publicKey) != KeySize {
		return nil, errKeySize
	}

	// Create an ephemeral key pair
	ephemeralPublicKey, ephemeralPrivateKey, err := GenerateKeys(e.rand)
	if err != nil {
		return nil, err
	}

	// encrypt plaintext
	ciphertext, err := xchacha20poly1305.Seal(
		computeSharedKey(e.publicKey, ephemeralPrivateKey),
		plaintext,
		crypto.ConcatByteSlices(e.publicKey, ephemeralPublicKey))
	if err != nil {
		return nil, err
	}

	header := []byte{crypto.Curve25519Xchacha20poly1305}

	return crypto.ConcatByteSlices(header, ephemeralPublicKey, ciphertext), nil
}

// Decrypt decrypts the ciphertext using the private Key
func (e encryptDecrypter) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(e.privateKey) != KeySize {
		return nil, errKeySize
	}

	if len(ciphertext) < cipherTextMinSize {
		return nil, errCipherTextSize
	}

	return xchacha20poly1305.Open(
		computeSharedKey(ciphertext[headerOffset:ephemeralPublicKeyOffset], e.privateKey),
		ciphertext[ephemeralPublicKeyOffset:],
		crypto.ConcatByteSlices(e.publicKey, ciphertext[headerOffset:ephemeralPublicKeyOffset]),
	)
}

// GenerateKeys creates a public/private key pair
func GenerateKeys(rand io.Reader) ([]byte, []byte, error) {
	publicKey := new([KeySize]byte)
	privateKey := new([KeySize]byte)
	if _, err := io.ReadFull(rand, privateKey[:]); err != nil {
		return nil, nil, err
	}

	curve25519.ScalarBaseMult(publicKey, privateKey)
	return publicKey[:], privateKey[:], nil
}

// New returns a new instance of the encryptDecrypter
func New(publicKey, privateKey []byte) crypto.EncryptDecrypter {
	return encryptDecrypter{publicKey: publicKey, privateKey: privateKey, rand: rand.Reader}
}

// computeSharedKey computes the shared key between peer's publicKey and privateKey.
func computeSharedKey(peerPublicKeyBytes, privateKeyBytes []byte) []byte {
	privateKey := new([KeySize]byte)
	copy(privateKey[:], privateKeyBytes)

	publicKey := new([KeySize]byte)
	copy(publicKey[:], peerPublicKeyBytes)

	sharedKey := new([KeySize]byte)
	curve25519.ScalarMult(sharedKey, privateKey, publicKey)

	return sharedKey[:]
}
