package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v3/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *KernelHandler) ServeAgentPluginArtifact(c *gin.Context) {
	h.serveAgentPluginRelease(c, false)
}

func (h *KernelHandler) ServeAgentPluginManifest(c *gin.Context) {
	h.serveAgentPluginRelease(c, true)
}

func (h *KernelHandler) serveAgentPluginRelease(c *gin.Context, manifestOnly bool) {
	nodeID, ok := kernelAgentNodeID(c)
	if !ok {
		kernelError(c, http.StatusUnauthorized, "node_auth_required", "authenticated node identity is required")
		return
	}
	sha256Hex, size, ok := parseAgentPluginDownloadAddress(c)
	if !ok {
		return
	}
	var download *service.AgentPluginReleaseDownload
	var err error
	download, err = service.LoadAuthorizedAgentPluginMetadata(h.db, nodeID, c.Param("plugin_id"), c.Param("version"))
	if err != nil {
		writeAgentPluginReleaseError(c, err)
		return
	}
	if manifestOnly {
		err = download.ValidateManifestAddress(sha256Hex, size)
	} else {
		err = download.ValidateArtifactAddress(sha256Hex, size)
	}
	if err != nil {
		kernelError(c, http.StatusBadRequest, "plugin_release_address_mismatch", err.Error())
		return
	}
	if !manifestOnly {
		download, err = service.LoadAgentPluginArtifactBlob(h.db, download)
		if err != nil {
			writeAgentPluginReleaseError(c, err)
			return
		}
	}

	contentSHA256 := download.Artifact.ArtifactSHA256
	if manifestOnly {
		contentSHA256 = download.ManifestHash
	}
	setAgentPluginReleaseHeaders(c, download, contentSHA256)
	if manifestOnly {
		c.Header("Content-Disposition", `attachment; filename="`+download.Release.PluginID+`-`+download.Release.Version+`.manifest.json"`)
		c.DataFromReader(http.StatusOK, int64(len(download.ManifestData)), "application/vnd.anixops.plugin-manifest+json", bytes.NewReader(download.ManifestData), nil)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+download.Release.PluginID+`-`+download.Release.Version+`.artifact"`)
	c.DataFromReader(http.StatusOK, download.Artifact.SizeBytes, "application/octet-stream", bytes.NewReader(download.Artifact.Data), nil)
}

func writeAgentPluginReleaseError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAgentPluginAssignmentDenied):
		kernelError(c, http.StatusForbidden, "plugin_release_not_assigned", err.Error())
	case errors.Is(err, gorm.ErrRecordNotFound):
		kernelError(c, http.StatusNotFound, "plugin_release_not_found", "assigned plugin release is unavailable")
	case errors.Is(err, service.ErrAgentPluginReleaseIntegrity):
		kernelError(c, http.StatusConflict, "plugin_release_integrity_failed", err.Error())
	default:
		kernelDBError(c, err)
	}
}

func kernelAgentNodeID(c *gin.Context) (uint, bool) {
	value, exists := c.Get("node_id")
	if !exists {
		return 0, false
	}
	switch nodeID := value.(type) {
	case uint:
		return nodeID, nodeID != 0
	case uint32:
		return uint(nodeID), nodeID != 0
	case int:
		return uint(nodeID), nodeID > 0
	case int64:
		return uint(nodeID), nodeID > 0
	default:
		return 0, false
	}
}

func parseAgentPluginDownloadAddress(c *gin.Context) (string, int64, bool) {
	query := c.Request.URL.Query()
	if len(query) != 2 || len(query["sha256"]) != 1 || len(query["size"]) != 1 {
		kernelError(c, http.StatusBadRequest, "invalid_plugin_release_address", "sha256 and size are required exactly once")
		return "", 0, false
	}
	sha256Hex := strings.ToLower(strings.TrimSpace(query.Get("sha256")))
	decoded, err := hex.DecodeString(sha256Hex)
	if err != nil || len(decoded) != 32 {
		kernelError(c, http.StatusBadRequest, "invalid_plugin_release_address", "sha256 must be a 64-character hexadecimal digest")
		return "", 0, false
	}
	size, err := strconv.ParseInt(query.Get("size"), 10, 64)
	if err != nil || size <= 0 {
		kernelError(c, http.StatusBadRequest, "invalid_plugin_release_address", "size must be a positive integer")
		return "", 0, false
	}
	return sha256Hex, size, true
}

func setAgentPluginReleaseHeaders(c *gin.Context, download *service.AgentPluginReleaseDownload, contentSHA256 string) {
	contentDigest, _ := hex.DecodeString(contentSHA256)
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	c.Header("ETag", `"sha256:`+strings.ToLower(contentSHA256)+`"`)
	c.Header("Digest", "sha-256="+base64.StdEncoding.EncodeToString(contentDigest))
	c.Header("X-AnixOps-Plugin-ID", download.Release.PluginID)
	c.Header("X-AnixOps-Plugin-Version", download.Release.Version)
	c.Header("X-AnixOps-Artifact-SHA256", strings.ToLower(download.Artifact.ArtifactSHA256))
	c.Header("X-AnixOps-Artifact-Size", strconv.FormatInt(download.Artifact.SizeBytes, 10))
	c.Header("X-AnixOps-Manifest-SHA256", download.ManifestHash)
	c.Header("X-AnixOps-Manifest-Size", strconv.FormatInt(int64(len(download.ManifestData)), 10))
	c.Header("X-AnixOps-Signature", download.Release.Signature)
	c.Header("X-AnixOps-Signature-Algorithm", "ed25519")
	c.Header("X-AnixOps-Publisher", download.Manifest.Publisher)
	c.Header("X-AnixOps-Key-ID", download.Release.TrustRootKeyID)
	c.Header("X-AnixOps-Plugin-API-Version", download.Manifest.APIVersion)
}
