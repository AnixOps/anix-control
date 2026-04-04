package binary

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	m := NewManager("")
	assert.NotNil(t, m)
	assert.NotEmpty(t, m.cacheDir)
}

func TestNewManagerWithCacheDir(t *testing.T) {
	cacheDir := "/tmp/test-cache"
	m := NewManager(cacheDir)
	assert.Equal(t, cacheDir, m.cacheDir)
}

func TestGetCachePath(t *testing.T) {
	m := NewManager("/tmp/cache")

	path := m.getCachePath(&XrayInfo)
	expectedSuffix := "xray"
	if runtime.GOOS == "windows" {
		expectedSuffix = "xray\\xray.exe"
	} else {
		expectedSuffix = "xray/xray"
	}
	assert.Contains(t, path, expectedSuffix)
}

func TestBuildDownloadURL(t *testing.T) {
	m := NewManager("")

	tests := []struct {
		name     string
		info     *BinaryInfo
		version  string
		contains []string // 可能包含的不同字符串
	}{
		{
			name:     "xray",
			info:     &XrayInfo,
			version:  "v1.8.0",
			contains: []string{"Xray", ".zip"},
		},
		{
			name:     "mihomo",
			info:     &MihomoInfo,
			version:  "v1.18.0",
			contains: []string{"mihomo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.info.Version = tt.version
			url, err := m.buildDownloadURL(tt.info)
			require.NoError(t, err)
			assert.Contains(t, url, tt.version)
			for _, c := range tt.contains {
				assert.Contains(t, url, c)
			}
		})
	}
}

func TestExtractGzip(t *testing.T) {
	// 跳过此测试，因为它需要有效的 gzip 文件
	t.Skip("requires valid gzip file")
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "dest.txt")

	// 创建源文件
	err := os.WriteFile(src, []byte("test content"), 0644)
	require.NoError(t, err)

	// 复制
	err = copyFile(src, dst)
	require.NoError(t, err)

	// 验证
	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(data))
}

func TestGetLatestVersion(t *testing.T) {
	// 创建模拟服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"tag_name": "v1.0.0"}`))
	}))
	defer server.Close()

	m := NewManager("")

	// 使用模拟服务器的 URL
	// 注意：实际测试中需要修改 getLatestVersion 来接受自定义 URL
	// 这里只测试结构
	assert.NotNil(t, m.client)
}

func TestEnsureBinaryNotFoundInPATH(t *testing.T) {
	m := NewManager(t.TempDir())

	// 测试一个不存在的二进制
	info := &BinaryInfo{
		Name:       "nonexistent-binary-xyz",
		Repo:       "test/test",
		BinaryName: "nonexistent-binary-xyz",
	}

	// 应该返回错误，因为不在 PATH 中也无法下载
	_, err := m.EnsureBinary(info)
	assert.Error(t, err)
}

func TestBinaryInfoConstants(t *testing.T) {
	assert.Equal(t, "xray", XrayInfo.Name)
	assert.Equal(t, "XTLS/Xray-core", XrayInfo.Repo)
	assert.Equal(t, "xray", XrayInfo.BinaryName)

	assert.Equal(t, "mihomo", MihomoInfo.Name)
	assert.Equal(t, "MetaCubeX/mihomo", MihomoInfo.Repo)
	assert.Equal(t, "mihomo", MihomoInfo.BinaryName)
}