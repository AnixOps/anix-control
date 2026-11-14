package handler

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/clientrelease"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/requestorigin"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ClientReleasesHandler hosts the ANX edition of the AnixOps Net client
// (internal/clientrelease, docs/reference/client-downloads.md): super
// administrators (or the anx release workflow with their admin API token)
// upload signed releases, and subscribers download the newest one with
// their subscription token.
type ClientReleasesHandler struct {
	db     func() *gorm.DB
	config func() *config.Config
	// servesHere is the subscription address's host rule; nil serves
	// every host.
	servesHere func(*gin.Context) bool
}

// NewClientReleasesHandler reads the database and the configuration on
// each request; the subscriber routes answer on the hosts subscriptions
// answer on.
func NewClientReleasesHandler(subscriptions *SubscribeHandler) *ClientReleasesHandler {
	h := &ClientReleasesHandler{db: database.Get, config: config.Get}
	if subscriptions != nil {
		h.servesHere = subscriptions.servesSubscriptionsHere
	}
	return h
}

func (h *ClientReleasesHandler) artifactDir() string {
	cfg := h.config()
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.AgentInstall.ArtifactDir)
}

func (h *ClientReleasesHandler) requireSuperAdmin(c *gin.Context) bool {
	ok, err := service.IsSuperAdmin(h.db(), kernelActorID(c))
	if err != nil {
		kernelDBError(c, err)
		return false
	}
	if !ok {
		kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may manage client releases")
		return false
	}
	return true
}

// Upload godoc
// @Summary Upload an ANX client release
// @Description Stores a release of the ANX edition of AnixOps Net (docs/reference/client-downloads.md) in agent_install.artifact_dir: multipart field files with SHA256SUMS, SHA256SUMS.sig (ssh-keygen -Y sign by the anx release key, namespace anx-release) and at least one file named anixops-net-anx-<version><suffix> (-windows-x64.msi, -windows-x64-portable.zip, -macos.pkg, -macos.dmg, -linux-amd64.deb, -linux-x64.tar.gz, -android-arm64.apk, -ios-testflight.url), each listed in SHA256SUMS. A release can come in several uploads with the same SHA256SUMS. 201 with the stored release; 400 client_release_invalid when it does not verify, 409 client_release_conflict for another SHA256SUMS of a stored version, 409 agent_install_unconfigured without an artifact directory. Super administrators only.
// @Tags Kernel
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Success 201 {object} map[string]any
// @Router /api/v4/kernel/client-releases [post]
func (h *ClientReleasesHandler) Upload(c *gin.Context) {
	if !h.requireSuperAdmin(c) {
		return
	}
	dir := h.artifactDir()
	if dir == "" {
		kernelError(c, http.StatusConflict, "agent_install_unconfigured", "set agent_install.artifact_dir to store client releases")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2*clientrelease.MaxFileBytes+1<<20)
	form, err := c.MultipartForm()
	if err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	defer func() { _ = form.RemoveAll() }()
	var sums []byte
	var signature string
	var uploads []clientrelease.Upload
	for _, header := range form.File["files"] {
		switch header.Filename {
		case clientrelease.SumsName, clientrelease.SignatureName:
			data, err := readSmallPart(header)
			if err != nil {
				kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			if header.Filename == clientrelease.SumsName {
				sums = data
			} else {
				signature = string(data)
			}
		default:
			uploads = append(uploads, clientrelease.Upload{
				Name: header.Filename,
				Open: func() (io.ReadCloser, error) { return header.Open() },
			})
		}
	}
	if sums == nil || signature == "" || len(uploads) == 0 {
		kernelError(c, http.StatusBadRequest, "invalid_request", "upload SHA256SUMS, SHA256SUMS.sig and at least one client file in the field files")
		return
	}
	version, err := clientrelease.Store(dir, sums, signature, uploads)
	switch {
	case errors.Is(err, clientrelease.ErrConflict):
		kernelError(c, http.StatusConflict, "client_release_conflict", err.Error())
		return
	case errors.Is(err, clientrelease.ErrInvalid), errors.Is(err, clientrelease.ErrNotFound):
		kernelError(c, http.StatusBadRequest, "client_release_invalid", err.Error())
		return
	case err != nil:
		kernelError(c, http.StatusInternalServerError, "client_release_store_failed", err.Error())
		return
	}
	release, err := clientrelease.Load(dir, version)
	if err != nil {
		kernelError(c, http.StatusInternalServerError, "client_release_store_failed", err.Error())
		return
	}
	kernelData(c, http.StatusCreated, release)
}

func readSmallPart(header *multipart.FileHeader) ([]byte, error) {
	file, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, clientrelease.MaxSumsBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > clientrelease.MaxSumsBytes {
		return nil, errors.New(header.Filename + " is too large")
	}
	return data, nil
}

// List godoc
// @Summary ANX client releases
// @Description The ANX client releases agent_install.artifact_dir holds, newest first, each with its files (name, platform, os, size, sha256) and TestFlight address; latest is the one subscribers are offered. configured is false without an artifact directory. Administrators read it.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/client-releases [get]
func (h *ClientReleasesHandler) List(c *gin.Context) {
	dir := h.artifactDir()
	releases := []clientrelease.Release{}
	for _, version := range clientrelease.Versions(dir) {
		if release, err := clientrelease.Load(dir, version); err == nil {
			releases = append(releases, release)
		}
	}
	latest := ""
	if release, err := clientrelease.Latest(dir); err == nil {
		latest = release.Version
	}
	kernelData(c, http.StatusOK, gin.H{
		"configured": dir != "", "latest": latest, "releases": releases, "platforms": clientrelease.Platforms,
	})
}

// Delete godoc
// @Summary Delete an ANX client release
// @Description Removes one stored ANX client release and its files; subscribers are then offered the newest one left. Super administrators only.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param version path string true "release version, for example 0.3.0-beta.6"
// @Success 204
// @Router /api/v4/kernel/client-releases/{version} [delete]
func (h *ClientReleasesHandler) Delete(c *gin.Context) {
	if !h.requireSuperAdmin(c) {
		return
	}
	err := clientrelease.Delete(h.artifactDir(), c.Param("version"))
	switch {
	case errors.Is(err, clientrelease.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", "no such client release")
		return
	case err != nil:
		kernelError(c, http.StatusInternalServerError, "client_release_delete_failed", err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

// subscriber resolves the subscription token of the path to its user;
// it answers the request itself and returns false when it cannot serve.
func (h *ClientReleasesHandler) subscriber(c *gin.Context) (string, bool) {
	if h.servesHere != nil && !h.servesHere(c) {
		c.Status(http.StatusNotFound)
		return "", false
	}
	token := c.Param("token")
	if token == "" || len(token) > 128 {
		c.Status(http.StatusNotFound)
		return "", false
	}
	var users []model.User
	if err := h.db().WithContext(c.Request.Context()).Where("token = ?", token).Limit(1).Find(&users).Error; err != nil {
		c.Status(http.StatusInternalServerError)
		return "", false
	}
	if len(users) == 0 {
		c.Status(http.StatusNotFound)
		return "", false
	}
	if !users[0].IsValid() {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"code": "subscription_inactive", "message": "the subscription is banned or expired"}})
		return "", false
	}
	return token, true
}

// Latest godoc
// @Summary The ANX client to download
// @Description The newest ANX edition of AnixOps Net for a subscriber: version, files (name, platform, os, size, sha256, url to download), testflight_url for iOS when the release has one, sha256sums_url and signature_url. The token is the subscription's, in the path. 404 for an unknown token or when Control holds no client release, 403 subscription_inactive for a banned or expired subscriber.
// @Tags Subscription
// @Produce json
// @Param token path string true "subscription token"
// @Success 200 {object} map[string]any
// @Router /s/{token}/client [get]
func (h *ClientReleasesHandler) Latest(c *gin.Context) {
	if _, ok := h.subscriber(c); !ok {
		return
	}
	release, err := clientrelease.Latest(h.artifactDir())
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "no_client_release", "message": "this Control holds no client release"}})
		return
	}
	// The files are under this request's own address
	// (/{subscribe_path}/{token}/client/...).
	base := requestorigin.Resolve(c.Request).BaseURL() + strings.TrimSuffix(c.Request.URL.Path, "/") + "/" + release.Version + "/"
	files := make([]gin.H, 0, len(release.Files))
	for _, file := range release.Files {
		files = append(files, gin.H{
			"name": file.Name, "platform": file.Platform, "os": file.OS, "size": file.Size, "sha256": file.SHA256,
			"url": base + file.Name,
		})
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"version": release.Version, "files": files, "testflight_url": release.TestFlightURL,
		"sha256sums_url": base + clientrelease.SumsName, "signature_url": base + clientrelease.SignatureName,
	}})
}

// Download godoc
// @Summary Download an ANX client file
// @Description One file of a stored ANX client release (a package, SHA256SUMS or SHA256SUMS.sig) for a subscriber, as an attachment. 404 for an unknown token or file, 403 subscription_inactive for a banned or expired subscriber.
// @Tags Subscription
// @Produce octet-stream
// @Param token path string true "subscription token"
// @Param version path string true "release version"
// @Param file path string true "file name"
// @Success 200
// @Router /s/{token}/client/{version}/{file} [get]
func (h *ClientReleasesHandler) Download(c *gin.Context) {
	if _, ok := h.subscriber(c); !ok {
		return
	}
	path, err := clientrelease.Path(h.artifactDir(), c.Param("version"), c.Param("file"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "private, max-age=3600")
	c.FileAttachment(path, c.Param("file"))
}
