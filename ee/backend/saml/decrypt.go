// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package saml

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"slices"

	"github.com/beevik/etree"
	"github.com/crewjam/saml/xmlenc"
)

// keyTransports are the algorithms the symmetric key of an encrypted
// assertion may be wrapped with. RSA PKCS#1 v1.5 is absent on purpose: its
// padding check is an oracle that decrypts the key of anyone who can submit
// responses, and every provider that encrypts supports OAEP.
var keyTransports = []string{
	"http://www.w3.org/2001/04/xmlenc#rsa-oaep-mgf1p",
	"http://www.w3.org/2009/xmlenc11#rsa-oaep",
}

// parseDecryptionKey reads the service provider's RSA private key, PKCS#8 or
// PKCS#1, from PEM.
func parseDecryptionKey(keyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("the service provider has no decryption key")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("the decryption key cannot be read")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("the decryption key is not an RSA key")
	}
	return key, nil
}

// decryptAssertion opens an EncryptedAssertion and returns the assertion in
// it. The key is used here and not kept.
func decryptAssertion(holder *etree.Element, keyPEM []byte) (*etree.Element, error) {
	key, err := parseDecryptionKey(keyPEM)
	if err != nil {
		return nil, err
	}
	data, err := required(holder, nsEncrypt, "EncryptedData")
	if err != nil {
		return nil, err
	}
	if err := checkKeyTransports(holder); err != nil {
		return nil, err
	}
	plaintext, err := decryptData(key, holder, data)
	if err != nil {
		return nil, fmt.Errorf("the encrypted assertion cannot be opened: %w", err)
	}
	return parseXML(plaintext)
}

// decryptData opens EncryptedData with the key wrapped inside it, or in an
// EncryptedKey beside it, which the specification allows as well.
func decryptData(key *rsa.PrivateKey, holder, data *etree.Element) ([]byte, error) {
	wrapped, err := optional(holder, nsEncrypt, "EncryptedKey")
	if err != nil {
		return nil, err
	}
	if wrapped == nil {
		return xmlenc.Decrypt(key, data) //nolint:wrapcheck // wrapped by the caller
	}
	symmetric, err := xmlenc.Decrypt(key, wrapped)
	if err != nil {
		return nil, fmt.Errorf("open the key: %w", err)
	}
	return xmlenc.Decrypt(symmetric, data) //nolint:wrapcheck // wrapped by the caller
}

// checkKeyTransports refuses a wrapped key whose algorithm is not allowed.
func checkKeyTransports(holder *etree.Element) error {
	for _, k := range holder.FindElements(".//EncryptedKey") {
		method, err := required(k, nsEncrypt, "EncryptionMethod")
		if err != nil {
			return err
		}
		if alg := method.SelectAttrValue("Algorithm", ""); !slices.Contains(keyTransports, alg) {
			return fmt.Errorf("the key transport algorithm %q is not accepted", alg)
		}
	}
	return nil
}
