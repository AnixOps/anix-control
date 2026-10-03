package grpc

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// artifactRelease is a signed official Agent release stored with its
// artifact and assigned to a node, and the install configuration the
// agent.plugin.install operation would carry.
type artifactRelease struct {
	manifest  service.PluginManifest
	canonical []byte
	artifact  []byte
	publicKey ed25519.PublicKey
	release   model.PluginRelease
	install   service.AgentPluginInstallConfig
}

// newReleaseSigner is the key of the kernel's one AnixOps trust root for a
// test: every release of the test is signed with it.
func newReleaseSigner(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return privateKey
}

// seedArtifactRelease registers a release of pluginID, signed by signer,
// with an artifact of size bytes and assigns it to nodeID.
func seedArtifactRelease(t *testing.T, db *gorm.DB, signer ed25519.PrivateKey, nodeID uint, pluginID string, size int) artifactRelease {
	t.Helper()
	require.NoError(t, service.EnsureKernelSchema(db))
	publicKey, privateKey := signer.Public().(ed25519.PublicKey), signer
	artifact := make([]byte, size)
	_, err := rand.Read(artifact)
	require.NoError(t, err)
	digest := sha256.Sum256(artifact)
	manifest := service.PluginManifest{
		ID: pluginID, Name: pluginID, Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, Architectures: []string{"linux-amd64"}, ArtifactSHA256: hex.EncodeToString(digest[:]),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: nodeID, ServiceScope: "telemetry", PluginID: pluginID, Role: "agent", DesiredVersion: manifest.Version, Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: pluginID, Target: "agent", DesiredVersion: manifest.Version, Enabled: true, State: "pending",
	}).Error)
	raw, err := service.BuildAgentPluginInstallConfig(db, nodeID, pluginID, manifest.Version)
	require.NoError(t, err)
	seeded := artifactRelease{manifest: manifest, canonical: canonical, artifact: artifact, publicKey: publicKey, release: *release}
	require.NoError(t, json.Unmarshal([]byte(raw), &seeded.install))
	return seeded
}

func (r artifactRelease) manifestAddress() *agentv1pb.PluginReleaseAddress {
	return &agentv1pb.PluginReleaseAddress{PluginId: r.install.PluginID, Version: r.install.Version, Sha256: r.install.Manifest.SHA256, Size: r.install.Manifest.Size}
}

func (r artifactRelease) artifactAddress() *agentv1pb.PluginReleaseAddress {
	return &agentv1pb.PluginReleaseAddress{PluginId: r.install.PluginID, Version: r.install.Version, Sha256: r.install.Artifact.SHA256, Size: r.install.Artifact.Size}
}

// downloadArtifact reads a whole DownloadPluginArtifact stream: the
// chunks' data joined, the first chunk's release, and the trailer.
func downloadArtifact(ctx context.Context, client agentv1pb.AgentArtifactsClient, address *agentv1pb.PluginReleaseAddress) ([]byte, *agentv1pb.PluginRelease, int, metadata.MD, error) {
	stream, err := client.DownloadPluginArtifact(ctx, &agentv1pb.DownloadPluginArtifactRequest{Artifact: address})
	if err != nil {
		return nil, nil, 0, nil, err
	}
	var data bytes.Buffer
	var release *agentv1pb.PluginRelease
	chunks := 0
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return data.Bytes(), release, chunks, stream.Trailer(), nil
		}
		if err != nil {
			return nil, nil, chunks, stream.Trailer(), err
		}
		if chunks == 0 {
			release = chunk.GetRelease()
		} else if chunk.GetRelease() != nil {
			return nil, nil, chunks, nil, errors.New("a later chunk repeats the release")
		}
		if chunk.GetOffset() != int64(data.Len()) {
			return nil, nil, chunks, nil, errors.New("chunk offsets are not contiguous")
		}
		data.Write(chunk.GetData())
		chunks++
	}
}

// verifyLikeTheAgent applies the checks the Agent applies to the HTTP
// download: sizes and digests of both documents against the install
// configuration, the manifest's ed25519 signature, and the artifact's
// digest in the manifest.
func verifyLikeTheAgent(t *testing.T, seeded artifactRelease, manifestJSON, artifact []byte, release *agentv1pb.PluginRelease) {
	t.Helper()
	manifestDigest := sha256.Sum256(manifestJSON)
	assert.Equal(t, seeded.install.Manifest.SHA256, hex.EncodeToString(manifestDigest[:]))
	assert.Equal(t, seeded.install.Manifest.Size, int64(len(manifestJSON)))
	artifactDigest := sha256.Sum256(artifact)
	assert.Equal(t, seeded.install.Artifact.SHA256, hex.EncodeToString(artifactDigest[:]))
	assert.Equal(t, seeded.install.Artifact.Size, int64(len(artifact)))
	signature, err := base64.StdEncoding.DecodeString(release.GetSignature())
	require.NoError(t, err)
	assert.True(t, ed25519.Verify(seeded.publicKey, manifestJSON, signature), "the manifest verifies against the release signature")
	var manifest service.PluginManifest
	require.NoError(t, json.Unmarshal(manifestJSON, &manifest))
	assert.Equal(t, hex.EncodeToString(artifactDigest[:]), manifest.ArtifactSHA256)
	assert.Equal(t, seeded.install.Manifest.Signature, release.GetSignature())
	assert.Equal(t, seeded.install.Manifest.KeyID, release.GetKeyId())
	assert.Equal(t, seeded.install.Manifest.Publisher, release.GetPublisher())
	assert.Equal(t, seeded.install.Manifest.APIVersion, release.GetPluginApiVersion())
	assert.Equal(t, "ed25519", release.GetSignatureAlgorithm())
	assert.Equal(t, seeded.install.Artifact.SHA256, release.GetArtifactSha256())
	assert.Equal(t, seeded.install.Artifact.Size, release.GetArtifactSize())
	assert.Equal(t, seeded.install.Manifest.SHA256, release.GetManifestSha256())
	assert.Equal(t, seeded.install.Manifest.Size, release.GetManifestSize())
}

// enrollNode enrolls an agent of node with a one-time credential and
// returns a connection that presents its certificate.
func enrollNode(t *testing.T, ctx context.Context, l *agentListener, node agentcontrol.AgentNode) (*grpc.ClientConn, *tls.Certificate) {
	t.Helper()
	credential, _, err := l.pki.CreateEnrollmentToken(ctx, agentpki.TokenRequest{Node: node, TTL: time.Hour})
	require.NoError(t, err)
	certificate, _, err := enrollForTest(t, ctx, l.dial(t, nil), credential)
	require.NoError(t, err)
	return l.dial(t, certificate), certificate
}

// refusalOf returns the gRPC code and x-anix-error-code of a refusal.
func refusalOf(err error, trailer metadata.MD) (codes.Code, string) {
	code := ""
	if values := trailer.Get(agentcontrol.MetadataErrorCode); len(values) == 1 {
		code = values[0]
	}
	return status.Code(err), code
}

// AgentArtifacts serves the node of the client certificate the releases
// assigned to it, byte for byte what the HTTP download serves, in chunks;
// the Agent's verification passes on them unchanged.
func TestAgentArtifactsServesAssignedReleasesByCertificate(t *testing.T) {
	previous := artifactChunkSize
	artifactChunkSize = 1000
	t.Cleanup(func() { artifactChunkSize = previous })
	l := startAgentListener(t, config.AgentMTLSRequired, true)
	db := database.Get()
	ctx := testContext(t)
	seeded := seedArtifactRelease(t, db, newReleaseSigner(t), l.proxy.ID, "artifact-telemetry", 2500)
	conn, _ := enrollNode(t, ctx, l, l.proxyNode())
	client := agentv1pb.NewAgentArtifactsClient(conn)

	manifest, err := client.GetPluginManifest(ctx, &agentv1pb.GetPluginManifestRequest{Manifest: seeded.manifestAddress()})
	require.NoError(t, err)
	assert.Equal(t, seeded.canonical, manifest.GetManifestJson())
	artifact, release, chunks, _, err := downloadArtifact(ctx, client, seeded.artifactAddress())
	require.NoError(t, err)
	assert.Equal(t, 3, chunks, "2500 bytes in chunks of 1000")
	assert.Equal(t, seeded.artifact, artifact)
	assert.Equal(t, manifest.GetRelease(), release, "both calls describe the release alike")
	assert.Equal(t, seeded.manifest.ID, release.GetPluginId())
	assert.Equal(t, seeded.manifest.Version, release.GetVersion())
	verifyLikeTheAgent(t, seeded, manifest.GetManifestJson(), artifact, release)

	// An uppercase digest names the same content, as on HTTP.
	upper := seeded.artifactAddress()
	upper.Sha256 = strings.ToUpper(upper.Sha256)
	again, _, _, _, err := downloadArtifact(ctx, client, upper)
	require.NoError(t, err)
	assert.Equal(t, seeded.artifact, again)
}

// Every refusal names its reason: no certificate (the node API key is not
// read), another node's certificate, a forward node, a release that is not
// assigned or not enabled, a wrong or malformed address, a release that no
// longer verifies, and a node at its download limit.
func TestAgentArtifactsRefusals(t *testing.T) {
	l := startAgentListener(t, config.AgentMTLSOptional, true)
	db := database.Get()
	ctx := testContext(t)
	signer := newReleaseSigner(t)
	seeded := seedArtifactRelease(t, db, signer, l.proxy.ID, "artifact-telemetry", 64)
	other := model.Node{Name: "other", Host: "127.0.0.2", APIKey: "other-key-placeholder", APIKeyHash: apiKeyHashForTest("other-key"), Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(&other).Error)
	unassigned := seedArtifactRelease(t, db, signer, other.ID, "other-telemetry", 64)

	conn, _ := enrollNode(t, ctx, l, l.proxyNode())
	client := agentv1pb.NewAgentArtifactsClient(conn)
	otherConn, _ := enrollNode(t, ctx, l, agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(other.ID)})
	forwardConn, _ := enrollNode(t, ctx, l, l.forwardNode())

	expectManifest := func(t *testing.T, ctx context.Context, client agentv1pb.AgentArtifactsClient, address *agentv1pb.PluginReleaseAddress, grpcCode codes.Code, errorCode string) {
		t.Helper()
		var trailer metadata.MD
		_, err := client.GetPluginManifest(ctx, &agentv1pb.GetPluginManifestRequest{Manifest: address}, grpc.Trailer(&trailer))
		require.Error(t, err)
		gotCode, gotErrorCode := refusalOf(err, trailer)
		assert.Equal(t, grpcCode, gotCode, "%v", err)
		assert.Equal(t, errorCode, gotErrorCode)
	}
	expectDownload := func(t *testing.T, client agentv1pb.AgentArtifactsClient, address *agentv1pb.PluginReleaseAddress, grpcCode codes.Code, errorCode string) {
		t.Helper()
		_, _, _, trailer, err := downloadArtifact(ctx, client, address)
		require.Error(t, err)
		gotCode, gotErrorCode := refusalOf(err, trailer)
		assert.Equal(t, grpcCode, gotCode, "%v", err)
		assert.Equal(t, errorCode, gotErrorCode)
	}

	t.Run("no certificate, even with the node API key", func(t *testing.T) {
		plain := agentv1pb.NewAgentArtifactsClient(l.dial(t, nil))
		expectManifest(t, legacyCredentials(ctx, l.proxy.ID, l.proxyKey), plain, seeded.manifestAddress(), codes.Unauthenticated, agentcontrol.ErrorCodeCertInvalid)
		expectDownload(t, plain, seeded.artifactAddress(), codes.Unauthenticated, agentcontrol.ErrorCodeCertInvalid)
	})
	t.Run("another node's certificate", func(t *testing.T) {
		wrong := agentv1pb.NewAgentArtifactsClient(otherConn)
		expectManifest(t, ctx, wrong, seeded.manifestAddress(), codes.PermissionDenied, agentcontrol.ErrorCodePluginReleaseNotAssigned)
		expectDownload(t, wrong, seeded.artifactAddress(), codes.PermissionDenied, agentcontrol.ErrorCodePluginReleaseNotAssigned)
		// ... which gets its own release.
		_, err := wrong.GetPluginManifest(ctx, &agentv1pb.GetPluginManifestRequest{Manifest: unassigned.manifestAddress()})
		require.NoError(t, err)
	})
	t.Run("x-node-id of another node", func(t *testing.T) {
		mismatched := metadata.AppendToOutgoingContext(ctx, agentcontrol.MetadataNodeID, "99")
		expectManifest(t, mismatched, client, seeded.manifestAddress(), codes.Unauthenticated, agentcontrol.ErrorCodeCertWrongNode)
	})
	t.Run("a forward node", func(t *testing.T) {
		forward := agentv1pb.NewAgentArtifactsClient(forwardConn)
		expectManifest(t, ctx, forward, seeded.manifestAddress(), codes.PermissionDenied, agentcontrol.ErrorCodePluginReleaseNotAssigned)
	})
	t.Run("a release assigned to another node", func(t *testing.T) {
		expectManifest(t, ctx, client, unassigned.manifestAddress(), codes.PermissionDenied, agentcontrol.ErrorCodePluginReleaseNotAssigned)
		expectDownload(t, client, unassigned.artifactAddress(), codes.PermissionDenied, agentcontrol.ErrorCodePluginReleaseNotAssigned)
	})
	t.Run("malformed addresses", func(t *testing.T) {
		for name, address := range map[string]*agentv1pb.PluginReleaseAddress{
			"none":       nil,
			"no plugin":  {Version: "1.0.0", Sha256: seeded.install.Manifest.SHA256, Size: 1},
			"short hash": {PluginId: "artifact-telemetry", Version: "1.0.0", Sha256: "abc", Size: 1},
			"no size":    {PluginId: "artifact-telemetry", Version: "1.0.0", Sha256: seeded.install.Manifest.SHA256},
		} {
			t.Run(name, func(t *testing.T) {
				expectManifest(t, ctx, client, address, codes.InvalidArgument, agentcontrol.ErrorCodePluginReleaseAddressInvalid)
				expectDownload(t, client, address, codes.InvalidArgument, agentcontrol.ErrorCodePluginReleaseAddressInvalid)
			})
		}
	})
	t.Run("an address that is not the release's content", func(t *testing.T) {
		wrongHash := seeded.manifestAddress()
		wrongHash.Sha256 = strings.Repeat("0", 64)
		expectManifest(t, ctx, client, wrongHash, codes.InvalidArgument, agentcontrol.ErrorCodePluginReleaseAddressMismatch)
		// The manifest's address does not name the artifact.
		expectDownload(t, client, seeded.manifestAddress(), codes.InvalidArgument, agentcontrol.ErrorCodePluginReleaseAddressMismatch)
		wrongSize := seeded.artifactAddress()
		wrongSize.Size++
		expectDownload(t, client, wrongSize, codes.InvalidArgument, agentcontrol.ErrorCodePluginReleaseAddressMismatch)
	})
	t.Run("a node at its download limit", func(t *testing.T) {
		previous := artifactDownloadsPerNode
		artifactDownloadsPerNode = 0
		t.Cleanup(func() { artifactDownloadsPerNode = previous })
		expectDownload(t, client, seeded.artifactAddress(), codes.ResourceExhausted, agentcontrol.ErrorCodePluginReleaseBusy)
	})
	t.Run("a stored artifact that no longer verifies", func(t *testing.T) {
		tampered := append([]byte(nil), seeded.artifact...)
		tampered[0] ^= 1
		require.NoError(t, db.Model(&model.PluginArtifact{}).Where("release_id = ?", seeded.release.ID).Update("data", tampered).Error)
		t.Cleanup(func() {
			require.NoError(t, db.Model(&model.PluginArtifact{}).Where("release_id = ?", seeded.release.ID).Update("data", seeded.artifact).Error)
		})
		expectDownload(t, client, seeded.artifactAddress(), codes.FailedPrecondition, agentcontrol.ErrorCodePluginReleaseIntegrityFailed)
	})
	t.Run("a disabled installation", func(t *testing.T) {
		require.NoError(t, db.Model(&model.PluginInstallation{}).Where("plugin_id = ?", "artifact-telemetry").Update("enabled", false).Error)
		t.Cleanup(func() {
			require.NoError(t, db.Model(&model.PluginInstallation{}).Where("plugin_id = ?", "artifact-telemetry").Update("enabled", true).Error)
		})
		expectManifest(t, ctx, client, seeded.manifestAddress(), codes.PermissionDenied, agentcontrol.ErrorCodePluginReleaseNotAssigned)
	})
	t.Run("a disabled node", func(t *testing.T) {
		require.NoError(t, db.Model(&model.Node{}).Where("id = ?", other.ID).Update("status", model.NodeStatusDisabled).Error)
		expectManifest(t, ctx, agentv1pb.NewAgentArtifactsClient(otherConn), unassigned.manifestAddress(), codes.PermissionDenied, agentcontrol.ErrorCodeCertRevoked)
	})

	// After the refusals the assigned release still downloads.
	data, _, _, _, err := downloadArtifact(ctx, client, seeded.artifactAddress())
	require.NoError(t, err)
	assert.Equal(t, seeded.artifact, data)
}

// artifacts.v1 is offered only to a session that authenticated a proxy
// node by certificate and lists it; the per-node limit counts downloads.
func TestAgentArtifactsOfferAndLimit(t *testing.T) {
	listed := []*agentv1pb.Capability{{Name: agentcontrol.CapabilityArtifacts, Version: agentcontrol.CapabilityVersionV1}}
	proxy := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}
	assert.True(t, servesArtifacts(agentPrincipal{Node: proxy, Certificate: true}, listed))
	assert.False(t, servesArtifacts(agentPrincipal{Node: proxy}, listed), "not to a node API key")
	assert.False(t, servesArtifacts(agentPrincipal{Node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 1}, Certificate: true}, listed))
	assert.False(t, servesArtifacts(agentPrincipal{Node: proxy, Certificate: true}, nil), "only when listed")

	server := NewAgentArtifactsGRPCServer(nil)
	assert.True(t, server.acquire(1))
	assert.True(t, server.acquire(1))
	assert.False(t, server.acquire(1), "two downloads per node")
	assert.True(t, server.acquire(2), "another node has its own")
	server.release(1)
	assert.True(t, server.acquire(1))
	server.release(1)
	server.release(1)
	server.release(2)
	assert.Empty(t, server.downloads)

	assert.Equal(t, codes.Unavailable, status.Code(pluginReleaseStatus(proxy, errors.New("database is down"))))
	assert.Equal(t, codes.Canceled, status.Code(pluginReleaseStatus(proxy, context.Canceled)))
	assert.Equal(t, codes.DeadlineExceeded, status.Code(pluginReleaseStatus(proxy, status.Error(codes.DeadlineExceeded, "late"))))
	assert.Equal(t, agentcontrol.ErrorCodePluginReleaseNotFound, refusalCode(pluginReleaseStatus(proxy, gorm.ErrRecordNotFound)))
}

// HelloAck lists artifacts.v1 for an enrolled agent that lists it, and not
// for one that authenticated with the node API key.
func TestAgentArtifactsOfferedOnCertificateSessions(t *testing.T) {
	l := startAgentListener(t, config.AgentMTLSOptional, true)
	ctx := testContext(t)
	open := func(conn *grpc.ClientConn, ctx context.Context) *agentv1pb.HelloAck {
		stream, err := agentv1pb.NewAgentControlServiceClient(conn).ControlStream(ctx)
		require.NoError(t, err)
		hello := validAgentHello(uint32(l.proxy.ID))
		hello.GetHello().Capabilities = append(hello.GetHello().Capabilities,
			&agentv1pb.Capability{Name: agentcontrol.CapabilityArtifacts, Version: agentcontrol.CapabilityVersionV1})
		require.NoError(t, stream.Send(hello))
		message, err := stream.Recv()
		require.NoError(t, err)
		require.NoError(t, stream.CloseSend())
		return message.GetHelloAck()
	}
	conn, _ := enrollNode(t, ctx, l, l.proxyNode())
	assert.True(t, agentcontrol.HasCapabilityVersion(open(conn, ctx).GetServerCapabilities(), agentcontrol.CapabilityArtifacts, agentcontrol.CapabilityVersionV1))
	legacy := open(l.dial(t, nil), legacyCredentials(ctx, l.proxy.ID, l.proxyKey))
	assert.False(t, agentcontrol.HasCapability(legacy.GetServerCapabilities(), agentcontrol.CapabilityArtifacts))
}
