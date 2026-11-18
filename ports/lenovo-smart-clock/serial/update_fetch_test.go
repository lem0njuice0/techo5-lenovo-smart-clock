//go:build cube_slot && cube_update_trial

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchSignedCubeBundle(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := make([]byte, 2048)
	root[1080], root[1081] = 0x53, 0xef
	boot := fakeBoot("a")
	rh, bh := sha256.Sum256(root), sha256.Sum256(boot)
	m := cubeUpdateManifest{Format: 1, Board: "cube", Version: "1.0", Sequence: 5, Slot: "a", SystemSize: int64(len(root)), SystemSHA256: hex.EncodeToString(rh[:]), BootSize: int64(len(boot)), BootSHA256: hex.EncodeToString(bh[:])}
	payload, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, payload)
	for _, scenario := range []string{"valid", "bad-signature", "corrupt-system", "missing-boot", "redirect", "downgrade", "existing-dir"} {
		t.Run(scenario, func(t *testing.T) {
			badSig := append([]byte(nil), sig...)
			if scenario == "bad-signature" {
				badSig[0] ^= 1
			}
			badRoot := append([]byte(nil), root...)
			if scenario == "corrupt-system" {
				badRoot[0] ^= 1
			}
			assets := map[string][]byte{"/feed/manifest-a.json": payload, "/feed/manifest-a.sig": badSig, "/feed/system-a.img": badRoot, "/feed/boot-a.img": boot}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scenario == "redirect" && r.URL.Path == "/feed/manifest-a.json" {
					http.Redirect(w, r, "/feed/other", http.StatusFound)
					return
				}
				if scenario == "missing-boot" && r.URL.Path == "/feed/boot-a.img" {
					http.NotFound(w, r)
					return
				}
				b, ok := assets[r.URL.Path]
				if !ok {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write(b)
			}))
			defer server.Close()
			client := server.Client()
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			dir := filepath.Join(t.TempDir(), "incoming")
			if scenario == "existing-dir" {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "sentinel"), []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			floor := uint64(5)
			if scenario == "downgrade" {
				floor = 6
			}
			err := fetchCubeBundle(context.Background(), client, server.URL+"/feed", dir, pub, "a", floor, int64(len(root)), int64(len(boot)))
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("unexpected fetch result: %v", err)
			}
			if scenario == "valid" {
				for _, name := range []string{"manifest.json", "manifest.sig", "system.img", "boot.img"} {
					if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
						t.Fatal(err)
					}
				}
			} else if scenario == "existing-dir" {
				if _, err := os.Stat(filepath.Join(dir, "sentinel")); err != nil {
					t.Fatal("existing content was modified")
				}
			} else if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("failed download left an incoming directory")
			}
		})
	}
	if _, err := cubeUpdateURL("http://example.invalid", "manifest-a.json"); err == nil {
		t.Fatal("accepted insecure update URL")
	}
	if _, err := cubeUpdateURL("https://u:p@example.invalid", "manifest-a.json"); err == nil {
		t.Fatal("accepted credentials in update URL")
	}
}
