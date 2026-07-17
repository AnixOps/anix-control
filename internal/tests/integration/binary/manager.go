package binary

import (
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var maxExtractedBinaryBytes int64 = 256 << 20

// BinaryInfo 二进制文件信息
type BinaryInfo struct {
	Name        string
	Repo        string
	BinaryName  string
	Version     string
	DownloadURL string
}

var (
	// XrayInfo Xray 二进制信息
	XrayInfo = BinaryInfo{
		Name:       "xray",
		Repo:       "XTLS/Xray-core",
		BinaryName: "xray",
	}

	// MihomoInfo Mihomo 二进制信息
	MihomoInfo = BinaryInfo{
		Name:       "mihomo",
		Repo:       "MetaCubeX/mihomo",
		BinaryName: "mihomo",
	}
)

// Manager 二进制文件管理器
type Manager struct {
	cacheDir string
	client   *http.Client
}

// NewManager 创建二进制文件管理器
func NewManager(cacheDir string) *Manager {
	if cacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = os.TempDir()
		}
		cacheDir = filepath.Join(homeDir, ".cache", "v2board-test")
	}
	return &Manager{
		cacheDir: cacheDir,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// EnsureBinary 确保二进制文件存在，如果不存在则下载
func (m *Manager) EnsureBinary(info *BinaryInfo) (string, error) {
	// 1. 先检查系统 PATH
	if path, err := m.findInPATH(info.BinaryName); err == nil {
		return path, nil
	}

	// 2. 检查缓存目录
	cachePath := m.getCachePath(info)
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}

	// 3. 下载到缓存目录
	if err := m.download(info); err != nil {
		return "", fmt.Errorf("failed to download %s: %w", info.Name, err)
	}

	return cachePath, nil
}

// findInPATH 在系统 PATH 中查找二进制文件
func (m *Manager) findInPATH(name string) (string, error) {
	// Windows 需要加 .exe 后缀
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	return path, nil
}

// getCachePath 获取缓存路径
func (m *Manager) getCachePath(info *BinaryInfo) string {
	binaryName := info.BinaryName
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	return filepath.Join(m.cacheDir, info.Name, binaryName)
}

// download 下载二进制文件
func (m *Manager) download(info *BinaryInfo) error {
	// 获取最新版本
	version, err := m.getLatestVersion(info.Repo)
	if err != nil {
		return fmt.Errorf("failed to get latest version: %w", err)
	}
	info.Version = version

	// 构建下载 URL
	downloadURL, err := m.buildDownloadURL(info)
	if err != nil {
		return err
	}

	fmt.Printf("Downloading %s %s from %s...\n", info.Name, version, downloadURL)

	// 创建缓存目录
	cachePath := m.getCachePath(info)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
		return err
	}

	// 下载文件
	tmpFile, err := os.CreateTemp("", info.Name+"-*.download")
	if err != nil {
		return err
	}
	tmpFileName := tmpFile.Name()
	tmpFileClosed := false
	defer func() {
		if !tmpFileClosed {
			if err := tmpFile.Close(); err != nil {
				log.Printf("close temporary download %s: %v", tmpFileName, err)
			}
		}
		if err := os.Remove(tmpFileName); err != nil && !os.IsNotExist(err) {
			log.Printf("remove temporary download %s: %v", tmpFileName, err)
		}
	}()

	resp, err := m.client.Get(downloadURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("close download response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temporary download: %w", err)
	}
	tmpFileClosed = true

	// 解压文件
	ext := strings.ToLower(filepath.Ext(downloadURL))
	switch ext {
	case ".zip":
		err = m.extractZip(tmpFile.Name(), cachePath, info.BinaryName)
	case ".gz":
		err = m.extractGzip(tmpFile.Name(), cachePath, info.BinaryName)
	default:
		// 直接复制
		err = copyFile(tmpFile.Name(), cachePath)
	}

	if err != nil {
		return err
	}

	// 设置可执行权限
	// #nosec G302 -- cached integration-test binaries must be executable and live in a private cache directory.
	if err := os.Chmod(cachePath, 0o750); err != nil {
		return fmt.Errorf("mark cached binary executable: %w", err)
	}

	fmt.Printf("Successfully installed %s to %s\n", info.Name, cachePath)
	return nil
}

// getLatestVersion 获取最新版本
func (m *Manager) getLatestVersion(repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	resp, err := m.client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("close latest-version response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get releases: %s", resp.Status)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if err := json.Unmarshal(data, &release); err != nil {
		return "", err
	}

	return release.TagName, nil
}

// buildDownloadURL 构建下载 URL
func (m *Manager) buildDownloadURL(info *BinaryInfo) (string, error) {
	goos := runtime.GOOS

	// 根据项目和平台构建文件名
	var filename string
	switch info.Name {
	case "xray":
		switch goos {
		case "windows":
			filename = "Xray-windows-64.zip"
		case "darwin":
			if runtime.GOARCH == "arm64" {
				filename = "Xray-macos-arm64.zip"
			} else {
				filename = "Xray-macos-64.zip"
			}
		default:
			filename = "Xray-linux-64.zip"
		}
	case "mihomo":
		arch := "amd64"
		if runtime.GOARCH == "arm64" {
			arch = "arm64"
		}
		switch goos {
		case "windows":
			filename = fmt.Sprintf("mihomo-windows-%s.zip", arch)
		case "darwin":
			filename = fmt.Sprintf("mihomo-darwin-%s.gz", arch)
		default:
			filename = fmt.Sprintf("mihomo-linux-%s.gz", arch)
		}
	default:
		return "", fmt.Errorf("unknown binary: %s", info.Name)
	}

	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s",
		info.Repo, info.Version, filename), nil
}

// extractZip 解压 ZIP 文件
func (m *Manager) extractZip(zipPath, destPath, binaryName string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			log.Printf("close zip archive %s: %v", zipPath, err)
		}
	}()

	// Windows 需要 .exe 后缀
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	for _, f := range r.File {
		// 查找目标二进制文件
		baseName := filepath.Base(f.Name)
		if baseName == binaryName {
			rc, err := f.Open()
			if err != nil {
				return err
			}

			out, closeRoot, err := createFileAtPath(destPath, 0o600)
			if err != nil {
				if closeErr := rc.Close(); closeErr != nil {
					return errors.Join(err, fmt.Errorf("close zip entry: %w", closeErr))
				}
				return err
			}

			copyErr := copyWithLimit(out, rc, maxExtractedBinaryBytes)
			closeErr := out.Close()
			rootErr := closeRoot()
			entryErr := rc.Close()
			if copyErr != nil || closeErr != nil || rootErr != nil || entryErr != nil {
				return errors.Join(copyErr, closeErr, rootErr, entryErr)
			}
			return nil
		}
	}

	return fmt.Errorf("binary %s not found in archive", binaryName)
}

// extractGzip 解压 GZIP 文件
func (m *Manager) extractGzip(gzPath, destPath, binaryName string) error {
	file, closeSourceRoot, err := openFileAtPath(gzPath)
	if err != nil {
		return err
	}

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		closeErr := file.Close()
		rootErr := closeSourceRoot()
		if closeErr != nil || rootErr != nil {
			return errors.Join(err, closeErr, rootErr)
		}
		return err
	}

	out, closeDestRoot, err := createFileAtPath(destPath, 0o600)
	if err != nil {
		readerErr := gzReader.Close()
		fileErr := file.Close()
		rootErr := closeSourceRoot()
		if readerErr != nil || fileErr != nil || rootErr != nil {
			return errors.Join(err, readerErr, fileErr, rootErr)
		}
		return err
	}

	copyErr := copyWithLimit(out, gzReader, maxExtractedBinaryBytes)
	outErr := out.Close()
	destRootErr := closeDestRoot()
	readerErr := gzReader.Close()
	fileErr := file.Close()
	sourceRootErr := closeSourceRoot()
	if copyErr != nil || outErr != nil || destRootErr != nil || readerErr != nil || fileErr != nil || sourceRootErr != nil {
		return errors.Join(copyErr, outErr, destRootErr, readerErr, fileErr, sourceRootErr)
	}
	return nil
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	srcFile, closeSourceRoot, err := openFileAtPath(src)
	if err != nil {
		return err
	}

	dstFile, closeDestRoot, err := createFileAtPath(dst, 0o600)
	if err != nil {
		srcErr := srcFile.Close()
		rootErr := closeSourceRoot()
		if srcErr != nil || rootErr != nil {
			return errors.Join(err, srcErr, rootErr)
		}
		return err
	}

	copyErr := copyWithLimit(dstFile, srcFile, maxExtractedBinaryBytes)
	dstErr := dstFile.Close()
	destRootErr := closeDestRoot()
	srcErr := srcFile.Close()
	sourceRootErr := closeSourceRoot()
	if copyErr != nil || dstErr != nil || destRootErr != nil || srcErr != nil || sourceRootErr != nil {
		return errors.Join(copyErr, dstErr, destRootErr, srcErr, sourceRootErr)
	}
	return nil
}

func openFileAtPath(path string) (*os.File, func() error, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, nil, err
	}
	file, err := root.Open(filepath.Base(path))
	if err != nil {
		rootErr := root.Close()
		if rootErr != nil {
			return nil, nil, errors.Join(err, rootErr)
		}
		return nil, nil, err
	}
	return file, root.Close, nil
}

func createFileAtPath(path string, perm os.FileMode) (*os.File, func() error, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, nil, err
	}
	file, err := root.OpenFile(filepath.Base(path), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		rootErr := root.Close()
		if rootErr != nil {
			return nil, nil, errors.Join(err, rootErr)
		}
		return nil, nil, err
	}
	return file, root.Close, nil
}

func copyWithLimit(dst io.Writer, src io.Reader, maxBytes int64) error {
	written, err := io.Copy(dst, io.LimitReader(src, maxBytes))
	if err != nil {
		return err
	}
	if written < maxBytes {
		return nil
	}

	var probe [1]byte
	n, err := src.Read(probe[:])
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if n > 0 {
		return fmt.Errorf("extracted binary exceeds %d bytes", maxBytes)
	}
	return nil
}
