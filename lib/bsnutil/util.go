package bsnutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// Functions:
//  audience_key = derive_key(audience, secret)
//  pseudonym = stable_encrypt(bsn, audience_key)
//  transport_token = random_encrypt(pseudonym, audience_key, nonce)
//
// stable_encrypt and random_encrypt are reversible functions'
//   - stable_encrypt always produces the same output for same input, it uses AES-ECB
//   - random_encrypt produces different output each time, it uses AES-GCM

const keyLength = 16 // AES-128

// TransportTokenToPseudonym converts a transport token to a pseudonym format.
func TransportTokenToPseudonym(token string, audience string) (string, error) {
	ivAndcipherText, err := hex.DecodeString(token)
	if err != nil {
		return "", fmt.Errorf("invalid token format: %w", err)
	}
	if len(ivAndcipherText) < aes.BlockSize {
		return "", fmt.Errorf("invalid token length")
	}
	iv := ivAndcipherText[:aes.BlockSize]
	cipherText := ivAndcipherText[aes.BlockSize:]
	if len(cipherText)%aes.BlockSize != 0 {
		return "", fmt.Errorf("invalid cipher text length")
	}
	block, err := aes.NewCipher(deriveKey(audience, keyLength))
	if err != nil {
		return "", err
	}
	decrypter := cipher.NewCBCDecrypter(block, iv)

	plainText := make([]byte, len(cipherText))
	decrypter.CryptBlocks(plainText, cipherText)
	return hex.EncodeToString(plainText), nil
}

// PseudonymToTransportToken converts a pseudonym to a transport token format.
func PseudonymToTransportToken(pseudonym string, audience string) (string, error) {
	iv := make([]byte, aes.BlockSize)
	_, _ = rand.Read(iv)
	block, err := aes.NewCipher(deriveKey(audience, keyLength))
	if err != nil {
		return "", err
	}
	encrypter := cipher.NewCBCEncrypter(block, iv)
	plainText, err := hex.DecodeString(pseudonym)
	if err != nil {
		return "", fmt.Errorf("invalid pseudonym format: %w", err)
	}
	cipherText := make([]byte, len(plainText))
	encrypter.CryptBlocks(cipherText, plainText)
	result := make([]byte, len(iv)+len(cipherText))
	copy(result, iv)
	copy(result[len(iv):], cipherText)
	return hex.EncodeToString(result), nil
}

func PseudonymToBSN(pseudonym string, audience string) (string, error) {
	data, err := hex.DecodeString(pseudonym)
	if err != nil {
		return "", err
	}
	key := deriveKey(audience, keyLength)
	decrypted, err := decryptAESECB(data, key)
	if err != nil {
		return "", err
	}
	return string(decrypted), nil
}

func BSNToPseudonym(bsn string, audience string) (string, error) {
	// AES-ECB to stable encrypt the BSN
	key := deriveKey(audience, keyLength)
	result, err := encryptAESECB([]byte(bsn), key)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(result), nil
}

// BSNToTransportToken converts a BSN to a transport token format, shorthand for BSNToPseudonym(PseudonymToTransportToken()).
func BSNToTransportToken(bsn string, audience string) (string, error) {
	pseudonym, err := BSNToPseudonym(bsn, audience)
	if err != nil {
		return "", err
	}
	token, err := PseudonymToTransportToken(pseudonym, audience)
	if err != nil {
		return "", err
	}
	return token, nil
}

// TransportTokenToBSN converts a transport token to a BSN format, shorthand for TransportTokenToPseudonym(PseudonymToBSN()).
func TransportTokenToBSN(token string, audience string) (string, error) {
	pseudonym, err := TransportTokenToPseudonym(token, audience)
	if err != nil {
		return "", err
	}
	bsn, err := PseudonymToBSN(pseudonym, audience)
	if err != nil {
		return "", err
	}
	return bsn, nil
}

func deriveKey(audience string, length int) []byte {
	const staticSecret = "secret-key"
	// Simple key derivation by XORing audience with static secret
	key := make([]byte, len(staticSecret))
	for i := 0; i < len(staticSecret); i++ {
		key[i] = staticSecret[i] ^ audience[i%len(audience)]
	}

	// Ensure key is of required length (16, 24, 32 bytes for AES)
	if len(key) < length {
		paddedKey := make([]byte, length)
		copy(paddedKey, key)
		key = paddedKey
	} else if len(key) > length {
		key = key[:length]
	}

	return key
}

func encryptAESECB(data, key []byte) ([]byte, error) {
	const keySize = 16
	if len(key) != keySize {
		return nil, fmt.Errorf("invalid key length %d, must be 16 bytes", len(key))
	}
	encrypter, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}
	// Pad data to multiple of keySize
	padding := keySize - (len(data) % keySize)
	if padding == keySize {
		padding = 0
	}
	paddedData := make([]byte, len(data)+padding)
	copy(paddedData, data)

	encrypted := make([]byte, len(paddedData))
	for bs, be := 0, keySize; bs < len(paddedData); bs, be = bs+keySize, be+keySize {
		encrypter.Encrypt(encrypted[bs:be], paddedData[bs:be])
	}

	return encrypted, nil
}

func decryptAESECB(data, key []byte) ([]byte, error) {
	const keySize = 16 // AES-128
	if len(key) != keySize {
		return nil, fmt.Errorf("invalid key length %d, must be 16 bytes", len(key))
	}
	if len(data)%keySize != 0 {
		return nil, fmt.Errorf("invalid data length %d, must be multiple of %d", len(data), keySize)
	}
	cipher, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}
	decrypted := make([]byte, len(data))
	for bs, be := 0, keySize; bs < len(data); bs, be = bs+keySize, be+keySize {
		cipher.Decrypt(decrypted[bs:be], data[bs:be])
	}
	// Remove padding (trailing zero bytes)
	i := len(decrypted)
	for i > 0 && decrypted[i-1] == 0 {
		i--
	}
	decrypted = decrypted[:i]
	return decrypted, nil
}
