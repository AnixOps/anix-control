package clientrelease

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/agentinstall"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// testKey stands in for the release key and signs like `ssh-keygen -Y sign`.
type testKey struct{ signer ssh.Signer }

func newTestKey(t *testing.T) testKey {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(private)
	require.NoError(t, err)
	return testKey{signer: signer}
}

func (k testKey) trust(t *testing.T) {
	t.Helper()
	previous := releaseKey
	releaseKey = string(ssh.MarshalAuthorizedKey(k.signer.PublicKey()))
	t.Cleanup(func() { releaseKey = previous })
}

func (k testKey) sign(t *testing.T, message []byte, namespace string) string {
	t.Helper()
	hash := sha512.Sum512(message)
	signature, err := k.signer.Sign(rand.Reader, append([]byte("SSHSIG"), ssh.Marshal(struct {
		Namespace     string
		Reserved      []byte
		HashAlgorithm string
		Hash          []byte
	}{namespace, nil, "sha512", hash[:]})...))
	require.NoError(t, err)
	blob := append([]byte("SSHSIG"), ssh.Marshal(struct {
		Version       uint32
		PublicKey     []byte
		Namespace     string
		Reserved      []byte
		HashAlgorithm string
		Signature     []byte
	}{1, k.signer.PublicKey().Marshal(), namespace, nil, "sha512", ssh.Marshal(signature)})...)
	return "-----BEGIN SSH SIGNATURE-----\n" + base64.StdEncoding.EncodeToString(blob) + "\n-----END SSH SIGNATURE-----\n"
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// release builds SHA256SUMS over files (name to contents) and its uploads.
func release(files map[string][]byte) ([]byte, []Upload) {
	var sums strings.Builder
	var uploads []Upload
	for _, platform := range Platforms {
		for name, data := range files {
			if !strings.HasSuffix(name, platform.Suffix) {
				continue
			}
			fmt.Fprintf(&sums, "%s  %s\n", digest(data), name)
			uploads = append(uploads, upload(name, data))
		}
	}
	return []byte(sums.String()), uploads
}

func upload(name string, data []byte) Upload {
	return Upload{Name: name, Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }}
}

func TestStoreAndLoad(t *testing.T) {
	key := newTestKey(t)
	key.trust(t)
	dir := t.TempDir()
	zip := []byte("zip")
	apk := []byte("apk")
	link := []byte("https://testflight.apple.com/join/abcdef\n")
	sums, uploads := release(map[string][]byte{
		"anixops-net-anx-0.3.0-beta.6-windows-x64-portable.zip": zip,
		"anixops-net-anx-0.3.0-beta.6-android-arm64.apk":        apk,
		"anixops-net-anx-0.3.0-beta.6-ios-testflight.url":       link,
	})
	signature := key.sign(t, sums, agentinstall.KernelSignatureNamespace)

	version, err := Store(dir, sums, signature, uploads)
	require.NoError(t, err)
	assert.Equal(t, "0.3.0-beta.6", version)

	got, err := Latest(dir)
	require.NoError(t, err)
	assert.Equal(t, "0.3.0-beta.6", got.Version)
	assert.Equal(t, "https://testflight.apple.com/join/abcdef", got.TestFlightURL)
	require.Len(t, got.Files, 3)
	// In the order of Platforms.
	assert.Equal(t, []string{"windows-portable", "android", "ios"}, []string{got.Files[0].Platform, got.Files[1].Platform, got.Files[2].Platform})
	assert.Equal(t, digest(zip), got.Files[0].SHA256)
	assert.Equal(t, int64(3), got.Files[0].Size)

	// The same SHA256SUMS may bring the rest later; another one may not.
	_, err = Store(dir, sums, signature, uploads[:1])
	require.NoError(t, err)
	other, otherUploads := release(map[string][]byte{"anixops-net-anx-0.3.0-beta.6-android-arm64.apk": []byte("other")})
	_, err = Store(dir, other, key.sign(t, other, agentinstall.KernelSignatureNamespace), otherUploads)
	require.ErrorIs(t, err, ErrConflict)

	require.NoError(t, Delete(dir, "0.3.0-beta.6"))
	_, err = Latest(dir)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, Delete(dir, "0.3.0-beta.6"), ErrNotFound)
}

func TestLatestIsTheNewestVersion(t *testing.T) {
	key := newTestKey(t)
	key.trust(t)
	dir := t.TempDir()
	for _, version := range []string{"0.3.0-beta.6", "0.3.0-beta.10", "0.2.9"} {
		sums, uploads := release(map[string][]byte{"anixops-net-anx-" + version + "-linux-x64.tar.gz": []byte(version)})
		_, err := Store(dir, sums, key.sign(t, sums, agentinstall.KernelSignatureNamespace), uploads)
		require.NoError(t, err)
	}
	assert.Equal(t, []string{"0.3.0-beta.10", "0.3.0-beta.6", "0.2.9"}, Versions(dir))
	got, err := Latest(dir)
	require.NoError(t, err)
	assert.Equal(t, "0.3.0-beta.10", got.Version)
}

func TestStoreRefusesBadUploads(t *testing.T) {
	key := newTestKey(t)
	key.trust(t)
	dir := t.TempDir()
	good := map[string][]byte{"anixops-net-anx-0.3.0-linux-amd64.deb": []byte("deb")}
	sums, uploads := release(good)
	signature := key.sign(t, sums, agentinstall.KernelSignatureNamespace)

	cases := map[string]func() error{
		"no files": func() error { _, err := Store(dir, sums, signature, nil); return err },
		"untrusted key": func() error {
			_, err := Store(dir, sums, newTestKey(t).sign(t, sums, agentinstall.KernelSignatureNamespace), uploads)
			return err
		},
		"wrong namespace": func() error {
			_, err := Store(dir, sums, key.sign(t, sums, "file"), uploads)
			return err
		},
		"changed file": func() error {
			_, err := Store(dir, sums, signature, []Upload{upload("anixops-net-anx-0.3.0-linux-amd64.deb", []byte("dub"))})
			return err
		},
		"unlisted file": func() error {
			_, err := Store(dir, sums, signature, []Upload{upload("anixops-net-anx-0.3.0-linux-x64.tar.gz", []byte("tar"))})
			return err
		},
		"not a client file": func() error {
			_, err := Store(dir, sums, signature, []Upload{upload("../anixops-net-anx-0.3.0-linux-amd64.deb", []byte("deb"))})
			return err
		},
		"two versions": func() error {
			s, u := release(map[string][]byte{
				"anixops-net-anx-0.3.0-linux-amd64.deb":  []byte("deb"),
				"anixops-net-anx-0.3.1-linux-x64.tar.gz": []byte("tar"),
			})
			_, err := Store(dir, s, key.sign(t, s, agentinstall.KernelSignatureNamespace), u)
			return err
		},
		"link not https": func() error {
			s, u := release(map[string][]byte{"anixops-net-anx-0.3.0-ios-testflight.url": []byte("http://example.com")})
			_, err := Store(dir, s, key.sign(t, s, agentinstall.KernelSignatureNamespace), u)
			return err
		},
	}
	for name, run := range cases {
		err := run()
		assert.True(t, errors.Is(err, ErrInvalid), "%s: %v", name, err)
	}
	// Nothing was stored, not even a temporary file.
	entries, err := os.ReadDir(filepath.Join(dir, Product))
	if err == nil {
		for _, entry := range entries {
			files, _ := os.ReadDir(filepath.Join(dir, Product, entry.Name()))
			assert.Empty(t, files, entry.Name())
		}
	}
	_, err = Latest(dir)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPathConfinesNames(t *testing.T) {
	dir := t.TempDir()
	path, err := Path(dir, "0.3.0", "anixops-net-anx-0.3.0-windows-x64.msi")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, Product, "0.3.0", "anixops-net-anx-0.3.0-windows-x64.msi"), path)
	for _, bad := range [][2]string{
		{"0.3.0", "anixops-net-anx-0.3.1-windows-x64.msi"},
		{"0.3.0", "../../etc/passwd"},
		{"..", SumsName},
		{"0.3.0/..", SumsName},
		{"0.3.0", "anixops-net-0.3.0-windows-x64.msi"},
	} {
		_, err := Path(dir, bad[0], bad[1])
		assert.Error(t, err, bad)
	}
	_, err = Path("", "0.3.0", SumsName)
	assert.Error(t, err)
}
