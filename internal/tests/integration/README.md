# Integration Test Framework

瀹屾暣鐨勮嚜鍔ㄥ寲娴嬭瘯妗嗘灦锛屾敮鎸佸畬鍏ㄦ湰鍦板寲鐨勭鍒扮浠ｇ悊娴嬭瘯銆?

## 鐩綍缁撴瀯

```
internal/tests/integration/
鈹溾攢鈹€ config/              # 閰嶇疆鐢熸垚妯″潡
鈹?  鈹溾攢鈹€ types.go         # 绫诲瀷瀹氫箟
鈹?  鈹溾攢鈹€ generator.go     # 鐢熸垚鍣ㄦ帴鍙?
鈹?  鈹溾攢鈹€ xray.go          # Xray JSON 閰嶇疆
鈹?  鈹溾攢鈹€ mihomo.go        # Mihomo YAML 閰嶇疆
鈹?  鈹斺攢鈹€ generator_test.go
鈹溾攢鈹€ clients/             # 瀹㈡埛绔鐞?
鈹?  鈹溾攢鈹€ client.go        # 瀹㈡埛绔帴鍙?
鈹?  鈹溾攢鈹€ xray_mihomo.go   # Xray/Mihomo 瀹炵幇
鈹?  鈹溾攢鈹€ manager.go       # 瀹㈡埛绔鐞嗗櫒
鈹?  鈹斺攢鈹€ client_test.go
鈹溾攢鈹€ runner/              # 娴嬭瘯鎵ц鍣?
鈹?  鈹溾攢鈹€ runner.go        # 娴嬭瘯杩愯鍣?
鈹?  鈹斺攢鈹€ runner_test.go
鈹溾攢鈹€ binary/              # 浜岃繘鍒惰嚜鍔ㄤ笅杞?
鈹?  鈹溾攢鈹€ manager.go       # GitHub Releases 涓嬭浇
鈹?  鈹斺攢鈹€ manager_test.go
鈹溾攢鈹€ mock/                # Mock 闈㈡澘鏈嶅姟鍣?
鈹?  鈹溾攢鈹€ server.go        # 妯℃嫙闈㈡澘 API
鈹?  鈹斺攢鈹€ server_test.go
鈹溾攢鈹€ echo/                # 鍐呯疆 Echo 鏈嶅姟鍣?
鈹?  鈹溾攢鈹€ server.go        # HTTP/TCP Echo
鈹?  鈹斺攢鈹€ server_test.go
鈹溾攢鈹€ local/               # 鏈湴铏氭嫙鐜
鈹?  鈹溾攢鈹€ environment.go   # 鏈湴娴嬭瘯鐜
鈹?  鈹斺攢鈹€ environment_test.go
鈹溾攢鈹€ e2e/                 # 绔埌绔畬鏁存祴璇?鉁?
鈹?  鈹斺攢鈹€ e2e_test.go      # 鍏ㄦ祦绋嬫祴璇?
鈹溾攢鈹€ integration_test.go
鈹斺攢鈹€ README.md
```

## 绔埌绔叏娴佺▼娴嬭瘯

### 鏋舵瀯鍥?

```
鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?
鈹?                    瀹屾暣 E2E 娴嬭瘯娴佺▼                                 鈹?
鈹?                                                                     鈹?
鈹? 鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?   鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?   鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?           鈹?
鈹? 鈹?娴嬭瘯绋嬪簭   鈹傗攢鈹€鈹€鈫掆攤 瀹㈡埛绔?Xray) 鈹傗攢鈹€鈹€鈫掆攤 鏈嶅姟绔?Xray) 鈹?           鈹?
鈹? 鈹?           鈹?   鈹?SOCKS5 浠ｇ悊  鈹?   鈹?鍗忚鏈嶅姟鍣?  鈹?           鈹?
鈹? 鈹?鍙戦€佽姹?  鈹?   鈹?:闅忔満绔彛    鈹?   鈹?:闅忔満绔彛    鈹?           鈹?
鈹? 鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?   鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?   鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹攢鈹€鈹€鈹€鈹€鈹€鈹€鈹?           鈹?
鈹?                                              鈫?                     鈹?
鈹?                                       鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?             鈹?
鈹?                                       鈹?Echo 鏈嶅姟鍣? 鈹?             鈹?
鈹?                                       鈹?HTTP 鍥炴樉    鈹?             鈹?
鈹?                                       鈹?:闅忔満绔彛    鈹?             鈹?
鈹?                                       鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?             鈹?
鈹?                                                                     鈹?
鈹? 鉁?瀹屽叏鏈湴鍖?- 涓嶉渶瑕佷簰鑱旂綉                                        鈹?
鈹? 鉁?鑷姩绔彛鍒嗛厤 - 閬垮厤鍐茬獊                                          鈹?
鈹? 鉁?鑷姩杩涚▼绠＄悊 - 鍚姩/鍋滄/娓呯悊                                    鈹?
鈹? 鉁?澶氬崗璁敮鎸?- Shadowsocks/VMess/VLESS/Trojan                     鈹?
鈹? 鉁?骞跺彂娴嬭瘯 - 楠岃瘉澶氳姹傚満鏅?                                       鈹?
鈹? 鉁?澶ф暟鎹祴璇?- 楠岃瘉鏁版嵁浼犺緭                                        鈹?
鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?
```

### 娴嬭瘯鍐呭

| 娴嬭瘯鍚嶇О | 璇存槑 |
|---------|------|
| `TestE2EShadowsocksFull` | Shadowsocks 瀹屾暣娴佺▼ |
| `TestE2EVMessFull` | VMess 瀹屾暣娴佺▼ |
| `TestE2EVLESSFull` | VLESS 瀹屾暣娴佺▼ |
| `TestE2ETrojanFull` | Trojan 瀹屾暣娴佺▼ |
| `TestE2EAllProtocols` | 鎵€鏈夊崗璁壒閲忔祴璇?|
| `TestE2EConcurrency` | 骞跺彂璇锋眰娴嬭瘯 |
| `TestE2ELargeData` | 澶ф暟鎹紶杈撴祴璇?|

### 浣跨敤鏂瑰紡

```go
func TestMyE2E(t *testing.T) {
    // 鍒涘缓娴嬭瘯濂椾欢
    suite := e2e.NewE2ETestSuite(t)
    if err := suite.Setup(); err != nil {
        t.Skipf("Setup failed: %v", err)
    }
    defer suite.Teardown()

    ctx := context.Background()

    // 鍚姩 Echo 鏈嶅姟鍣?
    echoPort, _ := suite.StartEchoServer(ctx)

    // 鑾峰彇浠ｇ悊绔彛
    proxyPort, _ := suite.GetFreePort()

    // 杩愯瀹屾暣娴嬭瘯
    result, err := suite.RunFullTest(ctx, proxyPort, echoPort, "shadowsocks")
    if err != nil {
        t.Fatal(err)
    }

    // 妫€鏌ョ粨鏋?
    if !result.Success {
        t.Errorf("Test failed: %s", result.Error)
    }
    t.Logf("Latency: %v", result.Latency)
}
```

### 娴嬭瘯鎶ュ憡

```
========== E2E Test Report ==========
Time: 2026-03-13T12:00:00Z
Total: 4, Passed: 3, Failed: 1
Pass Rate: 75.0%

Details:
  鉁?PASS [shadowsocks] Server:54321 鈫?Proxy:54322 鈫?Echo:54320 (latency: 10ms)
  鉁?PASS [vmess] Server:54323 鈫?Proxy:54324 鈫?Echo:54320 (latency: 15ms)
  鉁?PASS [vless] Server:54325 鈫?Proxy:54326 鈫?Echo:54320 (latency: 12ms)
  鉂?FAIL [trojan] Server:54327 鈫?Proxy:54328 鈫?Echo:54320 (latency: 0s)
       Error: connection timeout
======================================
```

## 杩愯娴嬭瘯

```bash
# 杩愯鎵€鏈夊崟鍏冩祴璇曪紙璺宠繃闇€瑕佷簩杩涘埗鐨勬祴璇曪級
go test -v -short ./internal/tests/integration/...

# 杩愯瀹屾暣 E2E 娴嬭瘯锛堥渶瑕佸畨瑁?xray锛?
go test -v ./internal/tests/integration/e2e/...

# 杩愯鐗瑰畾鍗忚娴嬭瘯
go test -v -run TestE2EShadowsocksFull ./internal/tests/integration/e2e/...

# 杩愯骞跺彂娴嬭瘯
go test -v -run TestE2EConcurrency ./internal/tests/integration/e2e/...
```

## 鍓嶇疆瑕佹眰

### 瀹夎 Xray

```bash
# Windows (浣跨敤 Scoop)
scoop install xray

# macOS
brew install xray

# Linux
curl -L https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-64.zip -o xray.zip
unzip xray.zip
sudo mv xray /usr/local/bin/
```

### 瀹夎 Mihomo锛堝彲閫夛級

```bash
# Windows
scoop install mihomo

# macOS
brew install mihomo
```

## 娴嬭瘯瑕嗙洊

| 妯″潡 | 娴嬭瘯鏁?| 瑕嗙洊鍐呭 |
|------|--------|---------|
| config | 6 | 閰嶇疆鐢熸垚銆佸崗璁敮鎸?|
| clients | 8 | 瀹㈡埛绔垱寤恒€佺鐞?|
| runner | 7 | 娴嬭瘯鎵ц銆佹姤鍛?|
| binary | 5 | 浜岃繘鍒朵笅杞姐€佺紦瀛?|
| mock | 6 | 闈㈡澘 API 妯℃嫙 |
| echo | 7 | Echo 鏈嶅姟鍣?|
| local | 8 | 鏈湴鐜 |
| e2e | 8 | 绔埌绔祴璇?|
| **鎬昏** | **55+** | - |

## CI/CD 闆嗘垚

```yaml
# .github/workflows/integration-test.yml
name: Integration Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'

      - name: Install Xray
        run: |
          curl -L https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-64.zip -o xray.zip
          unzip xray.zip
          sudo mv xray /usr/local/bin/

      - name: Run unit tests
        run: go test -v -short ./internal/tests/integration/...

      - name: Run E2E tests
        run: go test -v ./internal/tests/integration/e2e/...
```

## 鏀寔鐨勫崗璁?

| 鍗忚 | TLS | Reality | WebSocket | gRPC | QUIC |
|------|-----|---------|-----------|------|------|
| VMess | 鉁?| 鉁?| 鉁?| 鉁?| 鉂?|
| VLESS | 鉁?| 鉁?| 鉁?| 鉁?| 鉂?|
| Trojan | 鉁?| 鉂?| 鉁?| 鉁?| 鉂?|
| Shadowsocks | 鉂?| 鉂?| 鉂?| 鉂?| 鉂?|
| Hysteria2 | 鉁?| 鉂?| 鉂?| 鉂?| 鉁?|
| TUIC | 鉁?| 鉂?| 鉂?| 鉂?| 鉁?|

## 浣跨敤鏂规硶

### CLI 鍛戒护

```bash
# 鏋勫缓
go build -o integration-test ./cmd/integration-test/

# 鍩烘湰鐢ㄦ硶锛堣嚜鍔ㄤ笅杞戒簩杩涘埗锛?
./integration-test -host example.com -uuid your-uuid -download

# 瀹屾暣鍙傛暟
./integration-test \
  -host example.com \
  -port 443 \
  -protocol vless \
  -transport tcp \
  -tls reality \
  -uuid your-uuid \
  -email test@example.com \
  -sni www.google.com \
  -public-key your-public-key \
  -short-id your-short-id \
  -download \
  -timeout 30s \
  -output report.json

# 骞惰娴嬭瘯
./integration-test -host example.com -uuid your-uuid -parallel 4 -download

# 鍙祴璇?Xray
./integration-test -host example.com -uuid your-uuid -client xray -download
```

### 鑷姩涓嬭浇浜岃繘鍒?

娣诲姞 `-download` 鏍囧織鍚庯紝妗嗘灦浼氳嚜鍔細

1. 妫€鏌ョ郴缁?PATH 鏄惁鏈?xray/mihomo
2. 妫€鏌ョ紦瀛樼洰褰?`~/.cache/v2board-test/`
3. 浠?GitHub Releases 鑷姩涓嬭浇鏈€鏂扮増鏈?
   - Xray: `XTLS/Xray-core`
   - Mihomo: `MetaCubeX/mihomo`

### 浣滀负搴撲娇鐢?

```go
package main

import (
    "context"
    "time"

    "github.com/anixops/v2board/internal/tests/integration/binary"
    "github.com/anixops/v2board/internal/tests/integration/clients"
    "github.com/anixops/v2board/internal/tests/integration/config"
    "github.com/anixops/v2board/internal/tests/integration/runner"
)

func main() {
    // 鍒濆鍖栦簩杩涘埗绠＄悊鍣?
    binMgr := binary.NewManager("")
    clients.SetBinaryManager(&binaryAdapter{mgr: binMgr})

    // 鍒涘缓鏈嶅姟绔厤缃?
    server := config.ServerConfig{
        Host:      "example.com",
        Port:      443,
        Protocol:  config.ProtocolVLESS,
        Transport: config.TransportTCP,
        TLSType:   config.TLSReality,
        SNI:       "www.google.com",
        PublicKey: "your-public-key",
        ShortID:   "your-short-id",
    }

    // 鍒涘缓鐢ㄦ埛閰嶇疆
    user := config.UserConfig{
        UUID:  "your-uuid",
        Email: "test@example.com",
    }

    // 鍒涘缓杩愯鍣?
    r := runner.NewRunner(
        runner.WithTimeout(30*time.Second),
        runner.WithParallel(4),
    )

    // 杩愯娴嬭瘯
    report := r.Run(context.Background(), server, user)

    // 鎵撳嵃鎶ュ憡
    r.PrintReport()
}
```

### Mock 鏈嶅姟鍣?

鐢ㄤ簬鏈湴娴嬭瘯锛屾ā鎷熼潰鏉?API锛?

```go
package main

import (
    "context"
    "github.com/anixops/v2board/internal/tests/integration/mock"
)

func main() {
    srv := mock.NewServer(8080)

    // 娣诲姞娴嬭瘯鐢ㄦ埛
    srv.AddUser(&mock.MockUser{
        ID:    1,
        UUID:  "test-uuid",
        Email: "test@example.com",
    })

    // 鍚姩
    srv.Start(context.Background())
    defer srv.Stop(context.Background())

    // API 鍙敤:
    // GET  /health
    // GET  /api/v2/server/UniProxy/config
    // GET  /api/v2/server/UniProxy/user
    // POST /api/v2/server/UniProxy/push
    // POST /api/v2/server/UniProxy/alive
    // POST /api/v2/node/register
    // POST /api/v2/node/heartbeat
}
```

## 娴嬭瘯鍦烘櫙

榛樿娴嬭瘯鍦烘櫙锛?

| 鍚嶇О | 鍗忚 | 浼犺緭 | TLS |
|------|------|------|-----|
| vmess-tcp | VMess | TCP | 鏃?|
| vmess-ws-tls | VMess | WebSocket | TLS |
| vless-reality | VLESS | TCP | Reality |
| trojan-tls | Trojan | TCP | TLS |
| ss-tcp | Shadowsocks | TCP | 鏃?|
| hysteria2 | Hysteria2 | QUIC | TLS |

## 鐜鍙橀噺

杩愯瀹屾暣闆嗘垚娴嬭瘯闇€瑕佽缃細

```bash
export TEST_SERVER_HOST="your-server.com"
export TEST_SERVER_PORT="443"
export TEST_USER_UUID="your-uuid"
export TEST_PROTOCOL="vless"
export TEST_REALITY_PUBLIC_KEY="your-public-key"
export TEST_REALITY_SHORT_ID="your-short-id"
```

## 杩愯娴嬭瘯

```bash
# 杩愯鎵€鏈夊崟鍏冩祴璇曪紙璺宠繃闇€瑕佺綉缁滅殑娴嬭瘯锛?
go test -v -short ./internal/tests/integration/...

# 杩愯鎵€鏈夋祴璇曪紙鍖呮嫭瀹屾暣闆嗘垚娴嬭瘯锛?
go test -v ./internal/tests/integration/...

# 杩愯鐗瑰畾妯″潡娴嬭瘯
go test -v ./internal/tests/integration/config/...
go test -v ./internal/tests/integration/clients/...
go test -v ./internal/tests/integration/runner/...
go test -v ./internal/tests/integration/binary/...
go test -v ./internal/tests/integration/mock/...

# 娴嬭瘯瑕嗙洊鐜?
go test -cover ./internal/tests/integration/...
```

## CI/CD 闆嗘垚

椤圭洰宸查厤缃?GitHub Actions 鑷姩杩愯娴嬭瘯銆傚伐浣滄祦鏂囦欢浣嶄簬 `.github/workflows/integration-test.yml`銆?

## 杈撳嚭绀轰緥

```
========== Integration Test ==========
Server: example.com:443
Protocol: vless
Transport: tcp
TLS: reality
UUID: your-uuid
Scenarios: 6
Timeout: 30s
=======================================

========== Test Report ==========
Time: 2026-03-13T12:00:00Z
Total: 12, Passed: 10, Failed: 2
Pass Rate: 83.3%

Details:
  鉁?PASS [xray/vless] vless-reality (5.2s) - 120ms
  鉁?PASS [mihomo/vless] vless-reality (4.8s) - 98ms
  鉁?PASS [xray/vmess] vmess-tcp (3.1s) - 45ms
  鉂?FAIL [mihomo/vmess] vmess-ws-tls (2.5s) - 0s
       Error: connectivity test failed: connection timeout
=================================
```