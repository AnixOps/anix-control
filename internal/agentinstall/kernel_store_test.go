package agentinstall

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// testReleaseKey stands in for KernelReleaseKey and signs like
// `ssh-keygen -Y sign` (PROTOCOL.sshsig).
type testReleaseKey struct{ signer ssh.Signer }

func newTestReleaseKey(t *testing.T) testReleaseKey {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(private)
	require.NoError(t, err)
	return testReleaseKey{signer: signer}
}

func (k testReleaseKey) trust(t *testing.T) {
	t.Helper()
	previous := kernelReleaseKey
	kernelReleaseKey = string(ssh.MarshalAuthorizedKey(k.signer.PublicKey()))
	t.Cleanup(func() { kernelReleaseKey = previous })
}

func (k testReleaseKey) sign(t *testing.T, message []byte, namespace string) string {
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

func TestInstallScriptEmbedsTheKernelReleaseKey(t *testing.T) {
	// install.sh is signed as it is, so the key is written twice.
	assert.Contains(t, string(Script), `readonly KERNEL_RELEASE_KEY="`+KernelReleaseKey+`"`)
	assert.Contains(t, string(Script), `readonly KERNEL_NAMESPACE="`+KernelSignatureNamespace+`"`)
	_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(KernelReleaseKey))
	require.NoError(t, err)
}

func TestVerifyKernelSums(t *testing.T) {
	key := newTestReleaseKey(t)
	sums := []byte("abc  anx-node-0.1.0-x86_64-linux-musl.tar.gz\n")
	signature := key.sign(t, sums, KernelSignatureNamespace)
	// Only the built-in key is trusted.
	require.Error(t, VerifyKernelSums(sums, signature))
	key.trust(t)
	require.NoError(t, VerifyKernelSums(sums, signature))
	require.Error(t, VerifyKernelSums([]byte("abd"), signature))
	require.Error(t, VerifyKernelSums(sums, key.sign(t, sums, "file")))
	require.Error(t, VerifyKernelSums(sums, newTestReleaseKey(t).sign(t, sums, KernelSignatureNamespace)))
	require.Error(t, VerifyKernelSums(sums, testKernelSignature))
}

func TestStoreKernelRelease(t *testing.T) {
	key := newTestReleaseKey(t)
	key.trust(t)
	dir := t.TempDir()
	version := "0.2.0-rc.1"
	x86, arm := KernelAsset(version, "x86_64"), KernelAsset(version, "aarch64")
	x86Data, armData := []byte("x86 package"), []byte("arm package")
	sums := []byte(sha256Hex(x86Data) + "  " + x86 + "\n" + sha256Hex(armData) + "  " + arm + "\n")
	signature := key.sign(t, sums, KernelSignatureNamespace)

	// One package per upload: the second adds the other architecture.
	stored, err := StoreKernelRelease(dir, sums, signature, map[string][]byte{x86: x86Data})
	require.NoError(t, err)
	assert.Equal(t, version, stored)
	assert.Equal(t, version, KernelReleaseHeld(dir))
	_, err = StoreKernelRelease(dir, sums, signature, map[string][]byte{arm: armData})
	require.NoError(t, err)
	kernels, err := UpgradeKernels(dir, "https://panel.example.com")
	require.NoError(t, err)
	require.Len(t, kernels, 2)
	assert.Equal(t, signature, kernels[0].SumsSignature)
	entries, err := os.ReadDir(filepath.Join(dir, "anx-node", version))
	require.NoError(t, err)
	assert.Len(t, entries, 4, "no temporary file is left")

	refuse := func(target error, sums []byte, signature string, packages map[string][]byte) {
		t.Helper()
		_, err := StoreKernelRelease(dir, sums, signature, packages)
		require.Error(t, err)
		assert.True(t, errors.Is(err, target), err)
	}
	refuse(ErrKernelRelease, sums, newTestReleaseKey(t).sign(t, sums, KernelSignatureNamespace), map[string][]byte{x86: x86Data})
	refuse(ErrKernelRelease, sums, signature, map[string][]byte{x86: []byte("tampered")})
	refuse(ErrKernelReleaseFile, sums, signature, map[string][]byte{"anx-node.tar.gz": x86Data})
	refuse(ErrKernelReleaseFile, sums, signature, map[string][]byte{x86: x86Data, KernelAsset("0.2.0", "aarch64"): armData})
	refuse(ErrKernelReleaseFile, sums, signature, nil)
	other := []byte(sha256Hex(x86Data) + "  " + x86 + "\n")
	refuse(ErrKernelReleaseConflict, other, key.sign(t, other, KernelSignatureNamespace), map[string][]byte{x86: x86Data})
	stays, err := os.ReadFile(filepath.Join(dir, "anx-node", version, SumsAssetName))
	require.NoError(t, err)
	assert.Equal(t, sums, stays)
}
