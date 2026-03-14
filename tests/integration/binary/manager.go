package binary

import (
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

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
		homeDir, _ := os.UserHomeDir()
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
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return err
	}

	// 下载文件
	tmpFile, err := os.CreateTemp("", info.Name+"-*.download")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	resp, err := m.client.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return err
	}
	tmpFile.Close()

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
	os.Chmod(cachePath, 0755)

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
	defer resp.Body.Close()

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
		if goos == "windows" {
			filename = "Xray-windows-64.zip"
		} else if goos == "darwin" {
			if runtime.GOARCH == "arm64" {
				filename = "Xray-macos-arm64.zip"
			} else {
				filename = "Xray-macos-64.zip"
			}
		} else {
			filename = "Xray-linux-64.zip"
		}
	case "mihomo":
		arch := "amd64"
		if runtime.GOARCH == "arm64" {
			arch = "arm64"
		}
		if goos == "windows" {
			filename = fmt.Sprintf("mihomo-windows-%s.zip", arch)
		} else if goos == "darwin" {
			filename = fmt.Sprintf("mihomo-darwin-%s.gz", arch)
		} else {
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
	defer r.Close()

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
			defer rc.Close()

			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer out.Close()

			_, err = io.Copy(out, rc)
			return err
		}
	}

	return fmt.Errorf("binary %s not found in archive", binaryName)
}

// extractGzip 解压 GZIP 文件
func (m *Manager) extractGzip(gzPath, destPath, binaryName string) error {
	file, err := os.Open(gzPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, gzReader)
	return err
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}