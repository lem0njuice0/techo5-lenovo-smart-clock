//go:build cube_slot && cube_update_trial

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func cubeUpdateClient(ca []byte) (*http.Client, error) {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if len(ca) > 0 && !roots.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid update CA certificate")
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second}
	return &http.Client{Transport: transport, Timeout: 2 * time.Hour, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("update redirects are not allowed") }}, nil
}

func cubeUpdateURL(base, filename string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" {
		return "", errors.New("update source must be an uncredentialed HTTPS base URL")
	}
	if filename == "" || strings.ContainsAny(filename, "/\\?#") || filename == "." || filename == ".." {
		return "", errors.New("invalid update asset name")
	}
	u.Path = path.Join("/", u.Path, filename)
	return u.String(), nil
}

func fetchCubeBytes(ctx context.Context, client *http.Client, base, name string, limit int64) ([]byte, error) {
	address, err := cubeUpdateURL(base, name)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > limit {
		return nil, errors.New("invalid update response")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("update response too large")
	}
	return data, nil
}

func fetchCubeFile(ctx context.Context, client *http.Client, base, name, destination string, size int64, expected string) error {
	address, err := cubeUpdateURL(base, name)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || (resp.ContentLength >= 0 && resp.ContentLength != size) {
		return errors.New("invalid update asset response")
	}
	f, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, size+1))
	if copyErr != nil || n != size || hex.EncodeToString(h.Sum(nil)) != expected {
		f.Close()
		return errors.New("downloaded update asset did not match signed metadata")
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func writeCubeDownloadFile(name string, data []byte) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	n, err := f.Write(data)
	if err != nil {
		f.Close()
		return err
	}
	if n != len(data) {
		f.Close()
		return io.ErrShortWrite
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// fetchCubeBundle writes only to a newly created private incoming directory.
// Its caller supplies an HTTPS client with redirects disabled, authenticates
// the pinned key, holds the update lock and validates live partition geometry.
func fetchCubeBundle(ctx context.Context, client *http.Client, base, dir string, key ed25519.PublicKey, target string, floor uint64, systemSize, bootSize int64) (err error) {
	if client == nil || (target != "a" && target != "b") {
		return errors.New("invalid download request")
	}
	asset := func(prefix, ext string) string { return prefix + "-" + target + ext }
	payload, err := fetchCubeBytes(ctx, client, base, asset("manifest", ".json"), 16384)
	if err != nil {
		return err
	}
	sig, err := fetchCubeBytes(ctx, client, base, asset("manifest", ".sig"), ed25519.SignatureSize)
	if err != nil {
		return err
	}
	m, err := verifyCubeManifest(key, payload, sig)
	if err != nil {
		return err
	}
	if m.Slot != target || m.Sequence < floor || m.SystemSize != systemSize || m.BootSize > bootSize {
		return errors.New("update does not match inactive slot or accepted sequence")
	}
	if err := freeCubeDownloadSpace(filepath.Dir(dir), m.SystemSize, m.BootSize); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return err
	}
	names := []string{"manifest.json", "manifest.sig", "system.img", "boot.img"}
	defer func() {
		if err != nil {
			for _, name := range names {
				_ = os.Remove(filepath.Join(dir, name))
			}
			_ = os.Remove(dir)
		}
	}()
	if err = writeCubeDownloadFile(filepath.Join(dir, names[0]), payload); err != nil {
		return err
	}
	if err = writeCubeDownloadFile(filepath.Join(dir, names[1]), sig); err != nil {
		return err
	}
	if err = fetchCubeFile(ctx, client, base, asset("system", ".img"), filepath.Join(dir, names[2]), m.SystemSize, m.SystemSHA256); err != nil {
		return err
	}
	if err = fetchCubeFile(ctx, client, base, asset("boot", ".img"), filepath.Join(dir, names[3]), m.BootSize, m.BootSHA256); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err = d.Sync(); err != nil {
		return err
	}
	parent, err := os.Open(filepath.Dir(dir))
	if err != nil {
		return err
	}
	defer parent.Close()
	if err = parent.Sync(); err != nil {
		return err
	}
	return nil
}

func freeCubeDownloadSpace(dir string, systemSize, bootSize int64) error {
	if systemSize <= 0 || bootSize <= 0 || systemSize > 1024*1024*1024 || bootSize > 64*1024*1024 {
		return errors.New("invalid requested download sizes")
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return err
	}
	if st.Bsize <= 0 || uint64(st.Bavail) > ^uint64(0)/uint64(st.Bsize) {
		return errors.New("invalid shared-data filesystem capacity")
	}
	available := uint64(st.Bavail) * uint64(st.Bsize)
	if available < uint64(systemSize+bootSize+64*1024*1024) {
		return fmt.Errorf("insufficient shared-data space for update")
	}
	return nil
}
