//go:build cube_slot

package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
)

// A release signer must keep its private key outside the repository. A public
// verification key must be pinned by the eventual caller, never supplied by an
// update package. This parser deliberately accepts only canonical JSON.
type cubeUpdateManifest struct {
	Format       int    `json:"format"`
	Board        string `json:"board"`
	Version      string `json:"version"`
	Sequence     uint64 `json:"sequence"`
	Slot         string `json:"slot"`
	SystemSize   int64  `json:"system_size"`
	SystemSHA256 string `json:"system_sha256"`
	BootSize     int64  `json:"boot_size"`
	BootSHA256   string `json:"boot_sha256"`
}

var cubeUpdateVersion = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,63}$`)

func verifyCubeManifest(publicKey ed25519.PublicKey, payload, signature []byte) (cubeUpdateManifest, error) {
	var m cubeUpdateManifest
	if len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize || len(payload) == 0 || len(payload) > 16384 || !ed25519.Verify(publicKey, payload, signature) {
		return m, errors.New("update manifest signature invalid")
	}
	if err := json.Unmarshal(payload, &m); err != nil {
		return m, errors.New("invalid update manifest")
	}
	canonical, err := json.Marshal(m)
	if err != nil || !bytes.Equal(canonical, payload) {
		return m, errors.New("noncanonical update manifest")
	}
	if m.Format != 1 || m.Board != "cube" || m.Sequence == 0 || !cubeUpdateVersion.MatchString(m.Version) || (m.Slot != "a" && m.Slot != "b") || m.SystemSize <= 0 || m.SystemSize > 1024*1024*1024 || m.BootSize <= 0 || m.BootSize > 64*1024*1024 {
		return m, errors.New("unsupported update manifest")
	}
	for _, s := range []string{m.SystemSHA256, m.BootSHA256} {
		b, e := hex.DecodeString(s)
		if e != nil || len(b) != 32 || hex.EncodeToString(b) != s {
			return m, errors.New("invalid update digest")
		}
	}
	return m, nil
}
