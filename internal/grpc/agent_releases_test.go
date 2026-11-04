package grpc

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

// downloadRelease reads a whole DownloadRelease stream and its trailer.
func downloadRelease(ctx context.Context, client agentv1pb.AgentArtifactsClient, request *agentv1pb.DownloadReleaseRequest) ([]byte, metadata.MD, error) {
	stream, err := client.DownloadRelease(ctx, request)
	if err != nil {
		return nil, nil, err
	}
	var data bytes.Buffer
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return data.Bytes(), stream.Trailer(), nil
		}
		if err != nil {
			return nil, stream.Trailer(), err
		}
		if chunk.GetOffset() != request.GetOffset()+int64(data.Len()) {
			return nil, nil, errors.New("chunk offsets are not contiguous")
		}
		data.Write(chunk.GetData())
	}
}

// DownloadRelease serves the files of the artifact directory to proxy and
// forward nodes from any offset, and refuses names outside it.
func TestAgentArtifactsDownloadRelease(t *testing.T) {
	previous := artifactChunkSize
	artifactChunkSize = 1000
	t.Cleanup(func() { artifactChunkSize = previous })
	dir := t.TempDir()
	releaseArtifactDir = func() string { return dir }
	t.Cleanup(func() { releaseArtifactDir = func() string { return "" } })
	write := func(path string, size int) []byte {
		data := make([]byte, size)
		_, err := rand.Read(data)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, data, 0o644))
		return data
	}
	agentZip := write(filepath.Join(dir, "v5.1.0", "anix-agent-linux-64.zip"), 2500)
	kernel := write(filepath.Join(dir, "anx-node", "0.1.0-alpha.16", "anx-node-0.1.0-alpha.16-x86_64-linux-musl.tar.gz"), 1200)

	l := startAgentListener(t, true)
	ctx := testContext(t)
	for _, node := range []agentcontrol.AgentNode{l.proxyNode(), l.forwardNode()} {
		conn, _ := enrollNode(t, ctx, l, node)
		client := agentv1pb.NewAgentArtifactsClient(conn)

		got, _, err := downloadRelease(ctx, client, &agentv1pb.DownloadReleaseRequest{Kind: "agent", Version: "v5.1.0", Asset: "anix-agent-linux-64.zip"})
		require.NoError(t, err)
		assert.Equal(t, agentZip, got)
		got, _, err = downloadRelease(ctx, client, &agentv1pb.DownloadReleaseRequest{Kind: "agent", Version: "v5.1.0", Asset: "anix-agent-linux-64.zip", Offset: 1700})
		require.NoError(t, err)
		assert.Equal(t, agentZip[1700:], got, "a resumed download starts at the offset")
		got, _, err = downloadRelease(ctx, client, &agentv1pb.DownloadReleaseRequest{Kind: "anx-node", Version: "0.1.0-alpha.16", Asset: "anx-node-0.1.0-alpha.16-x86_64-linux-musl.tar.gz"})
		require.NoError(t, err)
		assert.Equal(t, kernel, got)
	}

	conn, _ := enrollNode(t, ctx, l, l.proxyNode())
	client := agentv1pb.NewAgentArtifactsClient(conn)
	for name, test := range map[string]struct {
		request *agentv1pb.DownloadReleaseRequest
		code    codes.Code
		reason  string
	}{
		"missing":   {&agentv1pb.DownloadReleaseRequest{Kind: "agent", Version: "v5.0.9", Asset: "anix-agent-linux-64.zip"}, codes.NotFound, agentcontrol.ErrorCodeReleaseNotFound},
		"traversal": {&agentv1pb.DownloadReleaseRequest{Kind: "agent", Version: "../v5.1.0", Asset: "anix-agent-linux-64.zip"}, codes.InvalidArgument, agentcontrol.ErrorCodeReleaseRequestInvalid},
		"kind":      {&agentv1pb.DownloadReleaseRequest{Kind: "plugin", Version: "v5.1.0", Asset: "anix-agent-linux-64.zip"}, codes.InvalidArgument, agentcontrol.ErrorCodeReleaseRequestInvalid},
		"asset":     {&agentv1pb.DownloadReleaseRequest{Kind: "anx-node", Version: "0.1.0-alpha.16", Asset: "config.json"}, codes.InvalidArgument, agentcontrol.ErrorCodeReleaseRequestInvalid},
	} {
		_, trailer, err := downloadRelease(ctx, client, test.request)
		code, reason := refusalOf(err, trailer)
		assert.Equal(t, test.code, code, name)
		assert.Equal(t, test.reason, reason, name)
	}
}
