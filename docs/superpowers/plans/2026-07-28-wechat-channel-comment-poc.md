# WeChat Channel Comment POC Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从源码构建一个只在隔离 Windows 虚拟机中运行的 `wx_channel_poc.exe`，搜索“青云装饰”的最多前 10 个公开作品，受限采集一级评论与二级回复，并输出经过秘密扫描的结构化 JSON、脱敏证据和清理回执。

**Architecture:** 新入口不调用 `app.Run()`，只组装回环 SunnyNet 代理、单客户端严格鉴权 WebSocket 桥、最小只读页面脚本、串行采集器和安全结果存储。原始微信响应只在内存中完成哈希、扫描和映射；普通磁盘输出只包含规范化数据与结构摘要。所有证书、驱动和进程规则由生命周期控制器按精确标识创建、记录并逆序清理。

**Tech Stack:** Go 1.24.3 module semantics、Windows/CGO、SunnyNet 本地 replace 模块、`github.com/coder/websocket`、标准库 `crypto/x509`/`crypto/aes`、PowerShell 5.1+、Node.js（仅测试最小注入脚本）。

---

## 不可违反的执行约束

- 不得运行仓库跟踪的 `wx_channel.exe`、`wx_channel_cloud.exe` 或 `wx_channel_radar.exe`。
- Task 2 完成前不得运行 `go test ./...`、任何会导入 `github.com/qtgolang/SunnyNet/SunnyNet` 的测试，或任何仓库二进制；Task 1 的 `internal/pocaudit` 静态测试是唯一例外。
- Task 17 的真实虚拟机验证必须等待实现、模拟集成、安全扫描全部通过，并获得用户对真实运行检查点的再次确认。
- 任何 Cookie、登录数据库、CA 私钥、Token、原始响应、评论正文或账号值都不得进入 Git、聊天或命令输出。
- 所有 PowerShell 文件删除必须使用已解析的精确 `-LiteralPath`，并验证路径位于任务目录；不得对仓库根、用户目录或未解析变量执行递归删除。

## 文件结构

### 新建

- `cmd/wx_channel_poc/main.go`：仅解析 `preflight`、`run`、`cleanup` 三个子命令并调用 POC 应用。
- `internal/pocaudit/source_test.go`：不导入 SunnyNet 的静态安全回归测试。
- `internal/poc/model.go`：稳定 JSON 类型、枚举和字段验证状态。
- `internal/poc/options.go`：固定关键词、限制、端口、等待时间及参数校验。
- `internal/poc/log.go`：固定事件名和字段白名单的安全日志接口及实现。
- `internal/poc/log_test.go`：日志字段拒绝与秘密扫描测试。
- `internal/poc/redact.go`：URL 清洗和普通输出秘密扫描。
- `internal/poc/evidence.go`：内存 SHA-256、结构摘要和可选 AES-GCM 临时证据。
- `internal/poc/store.go`：目录守卫、原子写入、检查点和 7 天保留清理。
- `internal/poc/atomic_windows.go`：Windows 原子替换实现。
- `internal/poc/atomic_other.go`：非 Windows 原子替换实现，仅供测试编译。
- `internal/poc/bridge.go`：单客户端、只读方法白名单、请求节流接口。
- `internal/poc/search.go`：搜索响应解析、去重、排名和耗尽判断。
- `internal/poc/comments.go`：一级评论、二级回复、上限和父子关系映射。
- `internal/poc/status.go`：运行、能力和覆盖三维结论。
- `internal/poc/wait.go`：300 秒人工等待和一次 300 秒延长。
- `internal/poc/ca.go`：单任务 CA 生成和内存 SunnyNet 证书管理器。
- `internal/poc/preflight.go`：Git 忽略、构建来源、端口和系统基线预检。
- `internal/poc/certstore_windows.go`：仅 CurrentUser Root 的证书安装、SHA-256 核验和精确删除。
- `internal/poc/certstore_other.go`：非 Windows 明确拒绝实现。
- `internal/poc/injector.go`：仅允许页面的 HTML 最小脚本注入。
- `internal/poc/proxy.go`：SunnyNet 回环代理和精确 `WeChatAppEx.exe` 规则包装。
- `internal/poc/runtime.go`：生命周期编排、信号处理、检查点和逆序清理。
- `internal/poc/runtime_windows.go`：Windows 驱动/证书真实适配器。
- `internal/poc/runtime_other.go`：非 Windows 运行拒绝。
- `internal/poc/app.go`：`preflight`、`run`、`cleanup` 应用服务。
- `internal/poc/testdata/search-page-1.json`：模拟搜索第一页。
- `internal/poc/testdata/search-page-end.json`：不足 10 个且明确耗尽的搜索末页。
- `internal/poc/testdata/comments-top.json`：一级评论、媒体正文和内嵌回复样本。
- `internal/poc/testdata/comments-replies.json`：独立二级回复分页样本。
- `internal/pocassets/embed.go`：只嵌入 POC 页面桥。
- `internal/pocassets/poc_api_client.js`：只读页面桥。
- `internal/pocassets/poc_api_client.test.js`：Node 沙箱白名单和无日志测试。
- `scripts/build-poc.ps1`：只从 `cmd/wx_channel_poc` 构建到 `.poc-build/`。
- `scripts/poc-security-audit.ps1`：源码、依赖图、监听和二进制秘密扫描。
- `scripts/poc-cleanup.ps1`：调用 POC 精确清理子命令并检查任务路径。
- `scripts/poc_scripts_test.go`：三个 POC 脚本的静态安全测试。
- `docs/runbooks/wechat-channel-comment-poc.md`：隔离虚拟机准备、运行、验收和销毁步骤。

### 修改

- `.gitignore`：忽略全部 POC 构建、秘密、运行和结果目录，并显式纳入三个 POC 脚本。
- `pkg/sunnynet/SunnyNet/SunnyNet.go`：移除导入时网络修改，添加显式旧入口调优、回环监听和可控驱动停止。
- `pkg/sunnynet/public/constobj.go`：删除静态根证书私钥常量。
- `pkg/sunnynet/src/CrossCompiled/windows.go`、`Linux.go`、`darwin.go`：添加驱动停止适配器。
- `pkg/sunnynet/src/nfapi/api.go`：释放 NFAPI 并在本次安装时注销驱动。
- `pkg/sunnynet/Resource/nfapi/PROVENANCE.md`：记录固定 NFAPI DLL/驱动来源、哈希、签名和许可证。
- `pkg/sunnynet/LICENSE`：保留固定 SunnyNet 依赖的 MIT 许可证。
- `internal/app/app.go`：旧入口显式调用网络调优，并改用 Sunny 实例的运行时证书。
- `internal/api/certificate.go`：旧证书端点改用 Sunny 实例的运行时证书。
- `internal/assets/embed.go`：移除静态 SunnyRoot 证书嵌入。
- `docs/superpowers/specs/2026-07-28-wechat-channel-comment-poc-design.md`：状态保持“已批准”。

### 删除

- `internal/assets/certs/SunnyRoot.key`：删除已公开且禁止继续使用的静态私钥。
- `internal/assets/certs/SunnyRoot.cer`：删除与静态私钥配套、已不再由运行时使用的证书。

## Task 0：建立受控 fork 和独立实施 worktree

**Files:** 无文件修改。

- [ ] **Step 1: 核对 GitHub CLI 身份，不在未认证状态创建远端资源**

Run:

```powershell
gh auth status
```

Expected: 当前 GitHub 主机显示已登录；若失败，停止并要求用户完成认证，不尝试读取或输出 Token。

- [ ] **Step 2: 在当前认证用户命名空间创建 fork**

Run:

```powershell
gh repo fork nobiyou/wx_channel --clone=false --remote=false
```

Expected: 返回 fork 已创建或已存在；不得在此步骤推送任何分支。

- [ ] **Step 3: 将远端关系改为 fork=`origin`、原作者=`upstream`**

Run:

```powershell
$forkOwner = gh api user --jq .login
git remote rename origin upstream
git remote add origin "https://github.com/$forkOwner/wx_channel.git"
git fetch --all --prune
git remote -v
```

Expected: `origin` 指向当前认证用户的 `wx_channel`，`upstream` 指向 `https://github.com/nobiyou/wx_channel.git`。

- [ ] **Step 4: 核对上游基线未漂移**

Run:

```powershell
git rev-parse upstream/main
git merge-base --is-ancestor 11d49cee1da9032230dc5c0eece79bcf03da3e82 upstream/main
```

Expected: 第一条输出当前上游提交；第二条退出码为 `0`。若基线不再是可达祖先，停止并重新审查差异。

- [ ] **Step 5: 创建独立实施 worktree 和功能分支**

Run:

```powershell
$repoRoot = git rev-parse --show-toplevel
$worktreeRoot = Join-Path (Split-Path -Parent $repoRoot) 'wechat-channel-comment-poc-impl'
git worktree add $worktreeRoot -b codex/wechat-channel-comment-poc codex/wechat-channel-comment-poc-design
git -C $worktreeRoot status --short --branch
```

Expected: 新 worktree 位于解析后的相邻目录，分支为 `codex/wechat-channel-comment-poc`，工作树干净。

## Task 1：先建立不会导入 SunnyNet 的静态安全测试

**Files:**

- Create: `internal/pocaudit/source_test.go`
- Modify: `.gitignore`
- Test: `internal/pocaudit/source_test.go`

- [ ] **Step 1: 写入 POC 忽略规则和脚本例外**

在 `.gitignore` 的运行目录段加入：

```gitignore
/.poc-build/
/.poc-tools/
/.poc-secrets/
/.poc-data/
/.poc-runtime/
/var/
*.poc-token
*.poc-cookie
*.raw-response.json
*.raw-evidence.enc
!scripts/build-poc.ps1
!scripts/poc-security-audit.ps1
!scripts/poc-cleanup.ps1
!scripts/poc_scripts_test.go
```

- [ ] **Step 2: 写静态失败测试，不导入任何 SunnyNet 包**

`internal/pocaudit/source_test.go` 使用标准库和 `git ls-files`，核心测试必须是：

```go
package pocaudit

import (
    "bytes"
    "go/ast"
    "go/parser"
    "go/token"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "testing"
)

func repoRoot(t *testing.T) string {
    t.Helper()
    cmd := exec.Command("git", "rev-parse", "--show-toplevel")
    out, err := cmd.Output()
    if err != nil { t.Fatalf("resolve repo root: %v", err) }
    return strings.TrimSpace(string(out))
}

func TestSunnyNetInitDoesNotTuneNetwork(t *testing.T) {
    root := repoRoot(t)
    path := filepath.Join(root, "pkg", "sunnynet", "SunnyNet", "SunnyNet.go")
    file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
    if err != nil { t.Fatal(err) }
    ast.Inspect(file, func(n ast.Node) bool {
        fn, ok := n.(*ast.FuncDecl)
        if !ok || fn.Name.Name != "init" || fn.Body == nil { return true }
        ast.Inspect(fn.Body, func(inner ast.Node) bool {
            call, ok := inner.(*ast.CallExpr)
            if !ok { return true }
            sel, ok := call.Fun.(*ast.SelectorExpr)
            if ok && sel.Sel.Name == "SetNetworkConnectNumber" {
                t.Errorf("SunnyNet init must not call SetNetworkConnectNumber")
            }
            return true
        })
        return false
    })
}

func TestTrackedSourceContainsNoPrivateKey(t *testing.T) {
    root := repoRoot(t)
    cmd := exec.Command("git", "ls-files", "-z")
    cmd.Dir = root
    out, err := cmd.Output()
    if err != nil { t.Fatal(err) }
    for _, raw := range bytes.Split(out, []byte{0}) {
        if len(raw) == 0 { continue }
        switch strings.ToLower(filepath.Ext(string(raw))) {
        case ".go", ".key", ".pem", ".cer", ".js", ".ps1", ".yaml", ".yml":
        default:
            continue
        }
        path := filepath.Join(root, filepath.FromSlash(string(raw)))
        data, err := os.ReadFile(path)
        if os.IsNotExist(err) { continue }
        if err != nil { t.Fatal(err) }
        rsaMarker := []byte(strings.Join([]string{"BEGIN", "RSA", "PRIVATE", "KEY"}, " "))
        genericMarker := []byte(strings.Join([]string{"BEGIN", "PRIVATE", "KEY"}, " "))
        if bytes.Contains(data, rsaMarker) || bytes.Contains(data, genericMarker) {
            t.Errorf("tracked private key marker: %s", raw)
        }
    }
}

func TestPOCRuntimeDirectoriesAreIgnored(t *testing.T) {
    root := repoRoot(t)
    for _, name := range []string{".poc-build/probe", ".poc-tools/probe", ".poc-secrets/probe", ".poc-data/probe", ".poc-runtime/probe", "var/probe"} {
        cmd := exec.Command("git", "check-ignore", "--quiet", name)
        cmd.Dir = root
        if err := cmd.Run(); err != nil { t.Errorf("not ignored: %s", name) }
    }
}
```

- [ ] **Step 3: 运行唯一安全的红灯测试**

Run:

```powershell
go test ./internal/pocaudit -run 'TestSunnyNetInitDoesNotTuneNetwork|TestTrackedSourceContainsNoPrivateKey|TestPOCRuntimeDirectoriesAreIgnored' -v
```

Expected: `TestPOCRuntimeDirectoriesAreIgnored` 通过；另外两项分别因 `SetNetworkConnectNumber` 和静态私钥失败。不得运行其他 Go 测试。

## Task 2：消除导入时系统修改和静态私钥

**Files:**

- Modify: `pkg/sunnynet/SunnyNet/SunnyNet.go`
- Modify: `pkg/sunnynet/public/constobj.go`
- Modify: `internal/app/app.go`
- Modify: `internal/api/certificate.go`
- Modify: `internal/assets/embed.go`
- Delete: `internal/assets/certs/SunnyRoot.key`
- Delete: `internal/assets/certs/SunnyRoot.cer`
- Test: `internal/pocaudit/source_test.go`
- Create: `pkg/sunnynet/Resource/nfapi/dll/win32/nfapi.dll`（固定第三方依赖，不执行）
- Create: `pkg/sunnynet/Resource/nfapi/dll/x64/nfapi.dll`（固定第三方依赖，不执行）
- Create: `pkg/sunnynet/Resource/nfapi/PROVENANCE.md`
- Create: `pkg/sunnynet/LICENSE`

- [ ] **Step 1: 将网络调优改为旧入口显式调用**

把 SunnyNet 顶部 `init()` 改为普通函数：

```go
// TuneNetworkStackForLegacy preserves the historical application behavior.
// Restricted callers such as wx_channel_poc must never call it.
func TuneNetworkStackForLegacy() {
    cores := runtime.NumCPU() - 1
    if cores < 1 { cores = 1 }
    runtime.GOMAXPROCS(cores)
    CrossCompiled.SetNetworkConnectNumber()
}
```

并在 `internal/app/app.go` 的 `(*App).Run` 第一条可执行语句加入：

```go
SunnyNet.TuneNetworkStackForLegacy()
```

POC 新入口不得引用该函数。

- [ ] **Step 2: 用进程内临时 CA 替代 SunnyNet 静态私钥**

删除 `public.RootCa` 和 `public.RootKey` 常量，把默认管理器初始化改为：

```go
var defaultManager = func() int {
    id := Certificate.CreateCertificate()
    manager := Certificate.LoadCertificateContext(id)
    if manager == nil { panic(errors.New("create certificate manager")) }
    if !manager.CreateCA("CN", "SunnyNet", "Runtime", "Beijing", "SunnyNet Runtime CA", "Beijing", 2048, 365) {
        panic(errors.New("create runtime CA"))
    }
    return id
}()
```

删除两个 `internal/assets/certs/SunnyRoot.*` 文件，并从 `internal/assets/embed.go` 删除 `CertData`。

- [ ] **Step 3: 让旧应用和旧证书端点使用当前 Sunny 实例证书**

在 `internal/app/app.go` 中把两处 `assets.CertData` 替换为一次获取的副本：

```go
runtimeCert := app.Sunny.ExportCert()
err := certificate.InstallCertificate(runtimeCert)
// 保存证书分支同样写 runtimeCert，不写私钥。
```

在 `internal/api/certificate.go` 中删除 `internal/assets` 依赖，安装和下载均使用：

```go
certData := s.sunny.ExportCert()
if len(certData) == 0 {
    response.Error(w, http.StatusInternalServerError, "runtime certificate unavailable")
    return
}
```

- [ ] **Step 4: 重新运行静态测试并确认转绿**

在干净克隆缺少 `go:embed` 所需 NFAPI DLL 时，只允许从 SunnyNet 官方仓库固定提交 `505b77b76da5872e8466a327dc6e574c42f7700c` 取得两个文件。核对 Git blob、SHA-256、文件大小和 Authenticode 状态，在 `PROVENANCE.md` 记录 DLL 未签名这一事实，并由 `TestPinnedNFAPIDependencies` 强制精确哈希。四个仓库已有 `.sys` 必须与同一提交的 Git blob 一致并通过 Microsoft Hardware Compatibility Publisher 签名验证。下载只补齐编译依赖，不加载任何 DLL 或驱动。

Run:

```powershell
go test ./internal/pocaudit -v
```

Expected: 全部 PASS；测试进程没有执行 `netsh`。

- [ ] **Step 5: 运行受影响的安全编译测试**

Run:

```powershell
$toolBin = (Resolve-Path '.poc-tools/tdm-gcc-10.3.0-2/bin').Path
$env:PATH = "$toolBin;$env:PATH"
$env:CC = Join-Path $toolBin 'gcc.exe'
$env:CXX = Join-Path $toolBin 'g++.exe'
$env:CGO_ENABLED = '1'
go test internal/api/certificate.go -count=1
```

Expected: PASS；不生成 `hardware_fingerprint.json`，不创建证书文件，不修改动态端口范围。固定 TDM-GCC 10.3.0 工具链来自项目现有构建文档指定的官方发布，安装器 SHA-256 为 `819C7A1F74D45AD04E10662E1A2C3124D13D9A2BCA508847692251242CD455C3`，开发时只解压在被忽略的 `.poc-tools/`，不修改系统 PATH。旧 `internal/api`/`internal/router` 整包会同时链接 Gopeed 的 crawshaw SQLite 与应用的 mattn SQLite，是上游既有重复符号缺陷；POC 依赖图必须排除二者，不以扩大范围修复旧下载应用。

- [ ] **Step 6: 提交第一条安全修复**

```powershell
git add .gitignore docs/superpowers/plans/2026-07-28-wechat-channel-comment-poc.md internal/pocaudit pkg/sunnynet/LICENSE pkg/sunnynet/Resource/nfapi pkg/sunnynet/SunnyNet/SunnyNet.go pkg/sunnynet/public/constobj.go internal/app/app.go internal/api/certificate.go internal/assets/embed.go internal/assets/certs
git commit -m "security: remove SunnyNet import side effects"
```

## Task 3：为 SunnyNet 增加回环监听和可控驱动释放

**Files:**

- Modify: `pkg/sunnynet/SunnyNet/SunnyNet.go`
- Modify: `pkg/sunnynet/src/CrossCompiled/windows.go`
- Modify: `pkg/sunnynet/src/CrossCompiled/Linux.go`
- Modify: `pkg/sunnynet/src/CrossCompiled/darwin.go`
- Modify: `pkg/sunnynet/src/nfapi/api.go`
- Create: `pkg/sunnynet/SunnyNet/safe_start_test.go`

- [ ] **Step 1: 写回环监听失败测试**

`pkg/sunnynet/SunnyNet/safe_start_test.go`：

```go
package SunnyNet

import "testing"

func TestSetLoopbackOnly(t *testing.T) {
    sunny := NewSunny()
    sunny.SetLoopbackOnly()
    if got := sunny.ListenHost(); got != "127.0.0.1" {
        t.Fatalf("ListenHost() = %q", got)
    }
}
```

Run:

```powershell
go test github.com/qtgolang/SunnyNet/SunnyNet -run TestSetLoopbackOnly -v
```

Expected: FAIL，提示方法不存在；此测试不会调用 `StartProcess`。

- [ ] **Step 2: 实现默认兼容、POC 显式收紧的监听地址**

在 `Sunny` 增加 `listenHost string`，`NewSunny()` 默认设为 `0.0.0.0`，并添加：

```go
func (s *Sunny) SetLoopbackOnly() *Sunny {
    s.listenHost = "127.0.0.1"
    return s
}

func (s *Sunny) ListenHost() string {
    if s.listenHost == "" { return "0.0.0.0" }
    return s.listenHost
}
```

`Start()` 中 TCP 和 UDP 地址统一改为：

```go
listenAddr := net.JoinHostPort(s.ListenHost(), strconv.Itoa(s.port))
tcpListen, err := net.Listen("tcp", listenAddr)
udpListenAddr, err := net.ResolveUDPAddr("udp", listenAddr)
```

- [ ] **Step 3: 实现显式 NFAPI 释放**

在 Windows `nfapi/api.go` 新增：

```go
func Shutdown(unregister bool) error {
    CancelAll()
    if apiNfInit {
        if _, err := Api.NfFree(); err != nil { return err }
        apiNfInit = false
    }
    if unregister && apiLoad {
        if _, err := Api.NfUnRegisterDriver(NF_DriverName); err != nil { return err }
    }
    return nil
}
```

Windows `CrossCompiled` 暴露 `NFapi_Shutdown(unregister bool) error` 调用它；Linux 和 macOS 返回 `nil`。SunnyNet 新增：

```go
func (s *Sunny) StopProcess(unregister bool) error {
    s.ProcessDelName("WeChatAppEx.exe")
    s.ProcessCancelAll()
    return CrossCompiled.NFapi_Shutdown(unregister)
}
```

不得调用 `ProcessALLName(true)`，不得删除基线中已存在的驱动。

- [ ] **Step 4: 运行嵌套模块测试和静态审计**

Run:

```powershell
go test github.com/qtgolang/SunnyNet/SunnyNet -run TestSetLoopbackOnly -count=1
go test ./internal/pocaudit -count=1
```

Expected: PASS。

- [ ] **Step 5: 提交 SunnyNet 安全表面**

```powershell
git add pkg/sunnynet/SunnyNet pkg/sunnynet/src/CrossCompiled pkg/sunnynet/src/nfapi/api.go
git commit -m "security: add loopback-only SunnyNet lifecycle"
```

## Task 4：定义稳定模型、固定限制和三维状态

**Files:**

- Create: `internal/poc/model.go`
- Create: `internal/poc/options.go`
- Create: `internal/poc/log.go`
- Create: `internal/poc/model_test.go`
- Test: `internal/poc/model_test.go`

- [ ] **Step 1: 写枚举和默认值失败测试**

```go
package poc

import (
    "bytes"
    "encoding/json"
    "testing"
    "time"
)

func TestDefaultOptionsAreFixedToApprovedSpec(t *testing.T) {
    got := DefaultOptions()
    if got.Keyword != "青云装饰" || got.Limits.Works != 10 ||
        got.Limits.TopLevelCommentsPerWork != 100 || got.Limits.RepliesPerWork != 200 {
        t.Fatalf("unexpected defaults: %+v", got)
    }
    if got.HumanWait.Timeout != 300*time.Second || got.HumanWait.Extension != 300*time.Second || got.HumanWait.MaxExtensions != 1 {
        t.Fatalf("unexpected human wait: %+v", got.HumanWait)
    }
    if got.RequestInterval != time.Second { t.Fatalf("interval=%s", got.RequestInterval) }
}

func TestDatasetSerializesMissingSourceFieldsAsNull(t *testing.T) {
    dataset := Dataset{SchemaVersion: SchemaVersion, Job: Job{Status: JobCompleted}, Comments: []Comment{{Level: 1}}}
    raw, err := json.Marshal(dataset)
    if err != nil { t.Fatal(err) }
    for _, want := range []string{`"comment_id":null`, `"parent_comment_id":null`, `"text":null`, `"account_id":null`} {
        if !bytes.Contains(raw, []byte(want)) { t.Errorf("missing %s in %s", want, raw) }
    }
}
```

Run: `go test ./internal/poc -run 'TestDefaultOptions|TestDatasetSerializes' -v`

Expected: FAIL，因为类型尚不存在。

- [ ] **Step 2: 创建模型枚举和顶层类型**

`internal/poc/model.go` 必须定义以下精确常量和公共结构：

```go
package poc

import "time"

const SchemaVersion = "wx-channel-comment-poc/1.0"

type JobStatus string
const (
    JobCompleted JobStatus = "completed"
    JobRequiresHuman JobStatus = "requires_human"
    JobPartial JobStatus = "partial"
    JobFailed JobStatus = "failed"
)

type CapabilityStatus string
const (
    CapabilityVerified CapabilityStatus = "verified"
    CapabilityVerifiedWithGaps CapabilityStatus = "verified_with_gaps"
    CapabilityInconclusive CapabilityStatus = "inconclusive"
    CapabilityFailed CapabilityStatus = "failed"
)

type CoverageStatus string
const (
    CoverageTargetMet CoverageStatus = "target_met"
    CoverageSourceExhausted CoverageStatus = "source_exhausted_below_target"
    CoverageIncomplete CoverageStatus = "incomplete"
)

type FieldStatus string
const (
    FieldPresent FieldStatus = "present"
    FieldMissingInSource FieldStatus = "missing_in_source"
    FieldInvalidFormat FieldStatus = "invalid_format"
    FieldNotApplicable FieldStatus = "not_applicable"
    FieldRedactedForSafety FieldStatus = "redacted_for_safety"
)

type Limits struct {
    Works int `json:"works"`
    TopLevelCommentsPerWork int `json:"top_level_comments_per_work"`
    RepliesPerWork int `json:"replies_per_work"`
}

type HumanWaitPolicy struct {
    Timeout time.Duration `json:"-"`
    Extension time.Duration `json:"-"`
    MaxExtensions int `json:"max_extensions_per_event"`
}

type Job struct {
    JobID string `json:"job_id"`
    Keyword string `json:"keyword"`
    Status JobStatus `json:"status"`
    CapabilityStatus CapabilityStatus `json:"capability_status"`
    CoverageStatus CoverageStatus `json:"coverage_status"`
    StartedAt time.Time `json:"started_at"`
    CompletedAt *time.Time `json:"completed_at"`
    Limits Limits `json:"limits"`
}

type MediaType struct { RawCode any `json:"raw_code"`; Normalized string `json:"normalized"` }
type SourceRef struct { Method string `json:"method"`; EvidenceRef string `json:"evidence_ref"`; Ordinal int `json:"ordinal"` }
type WorkLocator struct { Keyword string `json:"keyword"`; SearchRank int `json:"search_rank"`; SearchPage int `json:"search_page"`; IndexInPage int `json:"index_in_page"`; PublicURL *string `json:"public_url"` }
type PublicAccount struct { AccountID *string `json:"account_id"`; DisplayName *string `json:"display_name"`; AvatarURL *string `json:"avatar_url"` }
type Work struct {
    WorkID *string `json:"work_id"`; ObjectNonceID *string `json:"object_nonce_id"`; Title *string `json:"title"`
    Author PublicAccount `json:"author"`; MediaType MediaType `json:"media_type"`; Locator WorkLocator `json:"locator"`
    CollectionStatus string `json:"collection_status"`; TopLevelCommentCount int `json:"top_level_comment_count"`; ReplyCount int `json:"reply_count"`
    Truncation struct { Truncated bool `json:"truncated"`; Reasons []string `json:"reasons"` } `json:"truncation"`
    Source SourceRef `json:"source"`
}
type CommentContent struct { Text *string `json:"text"`; MediaType MediaType `json:"media_type"` }
type CommentTime struct { Raw *string `json:"raw"`; UnixSeconds *int64 `json:"unix_seconds"`; ISO8601 *string `json:"iso_8601"` }
type IPLocation struct { Label *string `json:"label"` }
type Comment struct {
    CommentID *string `json:"comment_id"`; WorkID *string `json:"work_id"`; Level int `json:"level"`
    ParentCommentID *string `json:"parent_comment_id"`; RootCommentID *string `json:"root_comment_id"`; RetrievalRootCommentID *string `json:"retrieval_root_comment_id"`
    Content CommentContent `json:"content"`; Account PublicAccount `json:"account"`; CreatedAt CommentTime `json:"created_at"`; IPLocation IPLocation `json:"ip_location"`; Source SourceRef `json:"source"`
}
type Dataset struct { SchemaVersion string `json:"schema_version"`; Job Job `json:"job"`; Works []Work `json:"works"`; Comments []Comment `json:"comments"` }
type FieldResult struct { Path string `json:"path"`; Status FieldStatus `json:"status"`; Applicable int `json:"applicable"`; Present int `json:"present"`; ReasonCode string `json:"reason_code"` }
type Validation struct { JobID string `json:"job_id"`; CapabilityStatus CapabilityStatus `json:"capability_status"`; CoverageStatus CoverageStatus `json:"coverage_status"`; Fields []FieldResult `json:"fields"`; ReasonCodes []string `json:"reason_codes"` }
type Counts struct { Works int `json:"works"`; TopLevelComments int `json:"top_level_comments"`; Replies int `json:"replies"` }
type Manifest struct {
    SchemaVersion string `json:"schema_version"`; JobID string `json:"job_id"`; Status JobStatus `json:"status"`
    CapabilityStatus CapabilityStatus `json:"capability_status"`; CoverageStatus CoverageStatus `json:"coverage_status"`
    Counts Counts `json:"counts"`; CleanupSuccess bool `json:"cleanup_success"`; CompletedAt *time.Time `json:"completed_at"`
    ReasonCodes []string `json:"reason_codes"`
}
type Checkpoint struct {
    SchemaVersion string `json:"schema_version"`; JobID string `json:"job_id"`; Phase string `json:"phase"`
    SearchMarker string `json:"search_marker"`; CurrentWorkRank int `json:"current_work_rank"`
    Works []Work `json:"works"`; Comments []Comment `json:"comments"`; SavedAt time.Time `json:"saved_at"`
}
type CleanupReceipt struct {
    JobID string `json:"job_id"`; Success bool `json:"success"`; Categories map[string]bool `json:"categories"`
    CompletedAt time.Time `json:"completed_at"`; ReasonCodes []string `json:"reason_codes"`
}
```

所有可能缺失的源字符串使用 `*string` 且不使用 `omitempty`，保证 JSON 输出为显式 `null`。`Checkpoint` 只允许保存恢复所需的规范化作品、评论、分页游标和计数，不得包含 Token、Cookie、证书路径、原始响应或原始错误字符串。

- [ ] **Step 3: 创建固定运行选项和严格校验**

```go
package poc

import (
    "errors"
    "net"
    "time"
)

type Options struct {
    Keyword string
    Limits Limits
    HumanWait HumanWaitPolicy
    RequestInterval time.Duration
    ProxyAddress string
    BridgeAddress string
    DataRoot string
    SecretsRoot string
    RuntimeRoot string
    BuildRoot string
    AckIsolatedVM bool
    AllowEncryptedRaw bool
}

func DefaultOptions() Options {
    return Options{
        Keyword: "青云装饰",
        Limits: Limits{Works: 10, TopLevelCommentsPerWork: 100, RepliesPerWork: 200},
        HumanWait: HumanWaitPolicy{Timeout: 300*time.Second, Extension: 300*time.Second, MaxExtensions: 1},
        RequestInterval: time.Second,
        ProxyAddress: "127.0.0.1:2025", BridgeAddress: "127.0.0.1:2026",
        DataRoot: ".poc-data", SecretsRoot: ".poc-secrets", RuntimeRoot: ".poc-runtime", BuildRoot: ".poc-build",
    }
}

func (o Options) ValidateForRun() error {
    if !o.AckIsolatedVM { return errors.New("isolated VM acknowledgement is required") }
    if o.Keyword != "青云装饰" { return errors.New("keyword must be 青云装饰") }
    if o.Limits != (Limits{Works:10, TopLevelCommentsPerWork:100, RepliesPerWork:200}) { return errors.New("limits differ from approved spec") }
    if o.HumanWait.Timeout != 300*time.Second || o.HumanWait.Extension != 300*time.Second || o.HumanWait.MaxExtensions != 1 { return errors.New("human wait policy differs from approved spec") }
    for _, addr := range []string{o.ProxyAddress, o.BridgeAddress} {
        host, _, err := net.SplitHostPort(addr)
        if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() { return errors.New("all listeners must be loopback") }
    }
    return nil
}
```

- [ ] **Step 4: 固定安全日志接口，不在此阶段写任何日志**

`internal/poc/log.go` 提前定义供 bridge 与 runtime 使用的窄接口和无输出实现：

```go
type SafeLogger interface {
    Event(name string, fields map[string]any) error
}

type DiscardLogger struct{}
func (DiscardLogger) Event(string, map[string]any) error { return nil }
```

Task 13 再增加经过字段白名单和 `ScanOrdinaryOutput` 的文件实现及测试。

- [ ] **Step 5: 运行模型测试并提交**

Run: `go test ./internal/poc -run 'TestDefaultOptions|TestDatasetSerializes' -count=1`

Expected: PASS。

```powershell
git add internal/poc/model.go internal/poc/options.go internal/poc/log.go internal/poc/model_test.go
git commit -m "feat: define POC data and status model"
```

## Task 5：实现秘密扫描、URL 脱敏和内存证据

**Files:**

- Create: `internal/poc/redact.go`
- Create: `internal/poc/redact_test.go`
- Create: `internal/poc/evidence.go`
- Create: `internal/poc/evidence_test.go`

- [ ] **Step 1: 写 URL 与秘密扫描红灯测试**

```go
func TestSafeURLDropsQueryAndFragment(t *testing.T) {
    got, status := SafeURL("https://img.example/a.jpg?token=secret&x=1#part")
    if got == nil || *got != "https://img.example/a.jpg" || status != FieldPresent { t.Fatalf("got=%v status=%s", got, status) }
}

func TestScanOutputRejectsCredentials(t *testing.T) {
    cases := [][]byte{
        []byte(`{"authorization":"Bearer abc"}`),
        []byte(`{"text":"-----BEGIN PRIVATE KEY-----"}`),
        []byte(`{"avatar_url":"https://x/a?session=s"}`),
    }
    for _, raw := range cases {
        if err := ScanOrdinaryOutput(raw); err == nil { t.Fatalf("accepted secret: %q", raw) }
    }
}

func TestObserveRawStoresStructureNotValues(t *testing.T) {
    recorder := NewEvidenceRecorder(nil)
    ev, err := recorder.Observe(1, "finderGetCommentList", []byte(`{"data":{"commentInfo":[{"commentId":"c-secret","content":"正文"}]}}`))
    if err != nil { t.Fatal(err) }
    encoded, _ := json.Marshal(ev)
    if bytes.Contains(encoded, []byte("c-secret")) || bytes.Contains(encoded, []byte("正文")) { t.Fatalf("raw value leaked: %s", encoded) }
    if ev.SourceResponseSHA256 == "" || ev.RecordCount != 1 { t.Fatalf("bad evidence: %+v", ev) }
}
```

Run: `go test ./internal/poc -run 'TestSafeURL|TestScanOutput|TestObserveRaw' -v`

Expected: FAIL，因为函数尚不存在。

- [ ] **Step 2: 实现 URL 清洗和普通输出扫描**

`SafeURL` 只接受 `http`/`https`，始终清除 `RawQuery` 和 `Fragment`。`ScanOrdinaryOutput` 必须拒绝以下不区分大小写模式：PEM 私钥标记、`Authorization`、`Bearer `、`Set-Cookie`、JSON 键 `cookie`/`token`/`session_token`/`private_key`，以及任何 URL 查询参数。函数签名固定为：

```go
func SafeURL(raw string) (*string, FieldStatus)
func ScanOrdinaryOutput(raw []byte) error
func RedactString(raw string) (*string, FieldStatus)
```

`RedactString` 对高置信秘密返回 `nil, FieldRedactedForSafety`；普通评论正文原样返回。

- [ ] **Step 3: 实现只保留结构的证据摘要**

```go
type SchemaField struct { Path string `json:"path"`; Type string `json:"type"`; Count int `json:"count,omitempty"` }
type RedactionCounts struct { CredentialKeys int `json:"credential_keys"`; QueryURLs int `json:"query_urls"`; PEMMarkers int `json:"pem_markers"` }
type Evidence struct {
    RequestSequence int `json:"request_sequence"`; Method string `json:"method"`; SourceResponseSHA256 string `json:"source_response_sha256"`
    ResponseBytes int `json:"response_bytes"`; RecordCount int `json:"record_count"`; Fields []SchemaField `json:"fields"`
    RedactionRuleVersion string `json:"redaction_rule_version"`; Redactions RedactionCounts `json:"redactions"`
}
type EvidenceRecorder struct { encrypted *EncryptedRawStore }
func NewEvidenceRecorder(encrypted *EncryptedRawStore) *EvidenceRecorder
func (r *EvidenceRecorder) Observe(sequence int, method string, raw []byte) (Evidence, error)
```

`Observe` 的顺序固定为：对原始载荷计算 SHA-256 → 内存敏感扫描计数 → 使用 `json.Decoder.UseNumber` 解析 → 递归收集排序后的字段路径、JSON 类型和数组长度 → 可选内存加密 → 返回摘要。不得把任何原始值放进 `Evidence`。

- [ ] **Step 4: 实现显式启用的 AES-256-GCM 临时原始证据**

```go
type EncryptedRawStore struct { dir string; key []byte }
func NewEncryptedRawStore(dir string, enabled bool) (*EncryptedRawStore, error)
func (s *EncryptedRawStore) Write(sequence int, raw []byte) error
func (s *EncryptedRawStore) Destroy() error
```

`enabled=false` 时返回 `nil, nil`。启用时生成 32 字节随机内存密钥；每份文件使用 12 字节随机 nonce，格式为 `nonce || aes_gcm_ciphertext`，权限 `0600`。密钥不写盘；`Destroy` 删除精确目录内的 `.enc` 文件并覆写内存 key slice 为零。

- [ ] **Step 5: 运行证据测试并提交**

Run: `go test ./internal/poc -run 'TestSafeURL|TestScanOutput|TestObserveRaw|TestEncryptedRaw' -count=1`

Expected: PASS，测试临时目录中没有明文响应。

```powershell
git add internal/poc/redact.go internal/poc/redact_test.go internal/poc/evidence.go internal/poc/evidence_test.go
git commit -m "feat: add redacted POC evidence pipeline"
```

## Task 6：实现 Git 守卫、原子存储、检查点和保留期清理

**Files:**

- Create: `internal/poc/store.go`
- Create: `internal/poc/store_test.go`
- Create: `internal/poc/atomic_windows.go`
- Create: `internal/poc/atomic_other.go`

- [ ] **Step 1: 写未忽略目录拒绝和敏感输出拒绝测试**

```go
func TestStoreRefusesUnignoredRoot(t *testing.T) {
    _, err := NewStore(StoreOptions{RepoRoot:t.TempDir(), DataRoot:"results", JobID:"job-1"})
    if err == nil || !strings.Contains(err.Error(), "git ignored") { t.Fatalf("err=%v", err) }
}

func TestStoreRejectsSecretBeforeAtomicReplace(t *testing.T) {
    store := newTestStore(t)
    err := store.WriteJSON("dataset.json", map[string]any{"token":"secret"})
    if err == nil { t.Fatal("expected secret rejection") }
    if _, statErr := os.Stat(filepath.Join(store.JobDir(), "dataset.json")); !os.IsNotExist(statErr) { t.Fatalf("unsafe file exists: %v", statErr) }
}
```

Run: `go test ./internal/poc -run 'TestStoreRefuses|TestStoreRejects' -v`

Expected: FAIL。

- [ ] **Step 2: 实现目录和 Git 忽略守卫**

`NewStore` 必须先解析仓库根和目标绝对路径，验证目标位于仓库根下，再对 `.poc-data/probe`、`.poc-secrets/probe`、`.poc-runtime/probe`、`.poc-build/probe`、`var/probe` 分别执行：

```go
cmd := exec.Command("git", "check-ignore", "--quiet", "--", relativeProbe)
cmd.Dir = repoRoot
```

任一退出非零时拒绝创建目录。创建后调用 Windows ACL 适配器移除继承，只授予当前用户 SID 完全控制。
所有根目录和任务目录还必须使用 `filepath.EvalSymlinks` 校验最终路径；Windows 适配器额外拒绝任何带 reparse-point 属性的路径分量。任何检查失败都不得创建、覆盖、移动或删除文件。

- [ ] **Step 3: 实现普通文件原子写入**

`WriteJSON` 固定流程：`json.MarshalIndent` → `ScanOrdinaryOutput` → 同目录 `os.CreateTemp` → `Chmod(0600)` → 写入并 `Sync` → 关闭 → `atomicReplace(temp,target)`。Windows 实现使用 `windows.MoveFileEx` 的 `MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH`；其他平台测试实现使用 `os.Rename`。

公开方法必须是：

```go
func (s *Store) WriteDataset(Dataset) error
func (s *Store) WriteValidation(Validation) error
func (s *Store) WriteManifest(Manifest) error
func (s *Store) WriteEvidence(Evidence) (string, error)
func (s *Store) SaveCheckpoint(Checkpoint) error
func (s *Store) WriteCleanupReceipt(CleanupReceipt) error
func (s *Store) DeleteExpired(now time.Time, maxAge time.Duration) ([]string, error)
func (s *Store) SecretsDir() string
```

`SecretsDir` 只返回已经完成忽略、边界、symlink/reparse-point 和 ACL 校验的当前任务秘密目录。`DeleteExpired` 只删除 `DataRoot` 下经 `filepath.Rel` 验证不含 `..` 且再次通过 symlink/reparse-point 校验的任务目录，并且只处理完成时间早于 `now-7*24h` 的 manifest。

- [ ] **Step 4: 验证检查点可覆盖且失败不破坏旧文件**

增加测试先写 checkpoint A，再注入 `atomicReplace` 错误写 checkpoint B，断言目标仍为 A；正常重试后为 B。

Run: `go test ./internal/poc -run 'TestStore|TestCheckpoint|TestDeleteExpired' -count=1`

Expected: PASS。

- [ ] **Step 5: 提交存储层**

```powershell
git add internal/poc/store.go internal/poc/store_test.go internal/poc/atomic_windows.go internal/poc/atomic_other.go
git commit -m "feat: add guarded atomic POC storage"
```

## Task 7：建立严格鉴权的单客户端只读 WebSocket 桥

**Files:**

- Create: `internal/poc/bridge.go`
- Create: `internal/poc/bridge_test.go`
- Create: `internal/pocassets/embed.go`
- Create: `internal/pocassets/poc_api_client.js`
- Create: `internal/pocassets/poc_api_client.test.js`

- [ ] **Step 1: 写服务端鉴权和白名单失败测试**

测试使用 `httptest.NewServer` 和 `coder/websocket.Dial`，覆盖：

```go
func TestBridgeRejectsQueryToken(t *testing.T)
func TestBridgeRejectsWrongOrigin(t *testing.T)
func TestBridgeAcceptsAuthSubprotocolWithoutEchoingSecret(t *testing.T)
func TestBridgeRejectsNonReadOnlyMethod(t *testing.T)
func TestBridgeStateDropsHrefAndUserAgent(t *testing.T)
```

合法请求头必须为 `Origin: https://channels.weixin.qq.com` 和子协议列表 `wx-poc-v1, auth.<base64url-token>`；服务端只协商并回显 `wx-poc-v1`。

Run: `go test ./internal/poc -run TestBridge -v`

Expected: FAIL。

- [ ] **Step 2: 实现固定协议和只读接口**

`bridge.go` 的公共表面固定为：

```go
var allowedMethods = map[string]struct{}{
    "finderSearch": {}, "finderGetCommentDetail": {}, "finderGetCommentList": {},
}
type BridgeState struct { PagePath string; Visible bool; Methods map[string]bool; LastSeen time.Time }
type Bridge interface {
    WaitReady(context.Context, []string) error
    Call(context.Context, string, any) ([]byte, error)
    State() BridgeState
    Close() error
}
type BridgeServer struct { /* one active client, request map, token hash, no raw-response logger */ }
func NewBridgeServer(token string, logger SafeLogger) *BridgeServer
func (s *BridgeServer) Handler() http.Handler
```

处理器必须同时验证远端 IP 为 loopback、Origin 精确匹配、URL 查询为空、子协议 Token 使用 `subtle.ConstantTimeCompare`。消息大小上限设为 4 MiB；只接受 `client_state`、`api_response`、`ping`。`Call` 在发送前检查白名单和页面声明的方法，不允许并发调用。

- [ ] **Step 3: 写最小页面桥，禁止复制上游全量客户端**

`poc_api_client.js` 只实现以下流程：

```javascript
(function () {
  'use strict';
  const config = window.__WX_CHANNEL_POC_CONFIG__;
  const allowed = new Set(['finderSearch', 'finderGetCommentDetail', 'finderGetCommentList']);
  const ws = new WebSocket(`ws://127.0.0.1:${config.port}/ws/api`, ['wx-poc-v1', `auth.${config.token}`]);

  function methods() {
    return {
      finderSearch: !!(window.WXU && WXU.API2 && typeof WXU.API2.finderSearch === 'function'),
      finderGetCommentDetail: !!(window.WXU && WXU.API && typeof WXU.API.finderGetCommentDetail === 'function'),
      finderGetCommentList: !!(window.WXU && WXU.API && typeof WXU.API.finderGetCommentList === 'function')
    };
  }
  function send(type, data) { if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({type, data})); }
  function state() { send('client_state', {pagePath: location.pathname, visible: !document.hidden, methods: methods(), timestamp: Date.now()}); }
  async function invoke(method, body) {
    if (!allowed.has(method)) throw new Error('method_not_allowed');
    if (method === 'finderSearch') return WXU.API2.finderSearch({query: body.keyword, scene: 19, requestId: String(Date.now()), lastBuffer: body.next_marker || '', lastBuff: body.next_marker || ''});
    if (method === 'finderGetCommentDetail') return WXU.API.finderGetCommentDetail({needObject:1,lastBuffer:'',scene:146,direction:2,identityScene:2,pullScene:6,objectid:String(body.object_id).split('_')[0],objectNonceId:body.nonce_id,encrypted_objectid:''});
    const payload = body.comment_id ? {direction:2,identityScene:2,objectId:body.object_id,rootCommentId:body.comment_id,lastBuffer:body.next_marker || undefined} : {finderBasereq:{scene:140,ctxInfo:{clientReportBuff:'{"entranceId":"1002"}'},objectBaseInfos:[]},objectId:body.object_id,objectNonceId:body.nonce_id,direction:2,identityScene:2,lastBuffer:body.next_marker || undefined,enterSessionId:String(Date.now())};
    return WXU.API.finderGetCommentList(payload);
  }
  ws.onopen = state;
  ws.onmessage = async (event) => {
    const message = JSON.parse(event.data);
    if (message.type === 'ping') { send('pong', {}); return; }
    if (message.type !== 'api_call') return;
    try { send('api_response', {id:message.data.id, data:await invoke(message.data.method, message.data.body), errCode:0}); }
    catch (error) {
      const text = String(error && error.message || '');
      const targetMismatch = text.includes('-70003') || text.includes('JSAPI_JSONPARSE_FAILED');
      send('api_response', {id:message.data.id, errCode:targetMismatch ? -70003 : 1011, errMsg:targetMismatch ? 'target_context_mismatch' : 'page_api_failed'});
    }
  };
  document.addEventListener('visibilitychange', state);
}());
```

脚本不得包含 `console.*`、`fetch`、下载或任何微信写方法名，也不得把请求 payload 附加到响应。

- [ ] **Step 4: 嵌入脚本并写 Node 沙箱测试**

`internal/pocassets/embed.go`：

```go
package pocassets
import _ "embed"
//go:embed poc_api_client.js
var APIClientJS []byte
```

Node 测试读取脚本文本，断言三个允许方法存在，并断言 `console.`、`fetch(`、`commentLike`、`commentPost`、`finderPost`、`publish`、`download`、`?token=` 不存在；还要断言 catch 分支只发送固定 `target_context_mismatch`/`page_api_failed`，不发送 `error.message`。错误对象只在内存中用于固定分类，绝不进入桥消息或日志。

Run:

```powershell
node internal/pocassets/poc_api_client.test.js
go test ./internal/poc -run TestBridge -count=1
```

Expected: 两者 PASS。

- [ ] **Step 5: 提交最小桥**

```powershell
git add internal/poc/bridge.go internal/poc/bridge_test.go internal/pocassets
git commit -m "feat: add read-only authenticated page bridge"
```

## Task 8：实现临时 CA、CurrentUser 证书库和只读预检

**Files:**

- Create: `internal/poc/ca.go`
- Create: `internal/poc/ca_test.go`
- Create: `internal/poc/preflight.go`
- Create: `internal/poc/preflight_test.go`
- Create: `internal/poc/certstore_windows.go`
- Create: `internal/poc/certstore_other.go`

- [ ] **Step 1: 写单任务 CA 和命令适配器红灯测试**

```go
func TestGenerateJobCAUsesUniqueCNAndSHA256(t *testing.T) {
    ca, err := GenerateJobCA("job-123", time.Unix(0, 0))
    if err != nil { t.Fatal(err) }
    if ca.Certificate.Subject.CommonName != "wx-channel-poc-job-123" { t.Fatalf("CN=%s", ca.Certificate.Subject.CommonName) }
    if ca.SHA256Fingerprint == "" || bytes.Contains(ca.CertPEM, ca.KeyPEM) { t.Fatal("invalid CA material") }
}

func TestPreflightRejectsMissingIsolationAck(t *testing.T) {
    report, err := NewPreflight(fakeRunner{}, fakeVMDetector{isolated:true}).Run(context.Background(), DefaultOptions())
    if err == nil || report.Passed { t.Fatalf("report=%+v err=%v", report, err) }
}

func TestCertificateRemovalVerifiesSHA256BeforeDelete(t *testing.T) {
    runner := &recordingRunner{output: []byte("different-sha256")}
    store := newCertificateStore(runner)
    if err := store.RemoveBySHA256(context.Background(), "expected-sha256"); err == nil { t.Fatal("expected mismatch") }
    if runner.calledRemove { t.Fatal("must not remove mismatched certificate") }
}
```

Run: `go test ./internal/poc -run 'TestGenerateJobCA|TestPreflight|TestCertificateRemoval' -v`

Expected: FAIL。

- [ ] **Step 2: 生成 3072 位单任务 CA 并只把私钥写入秘密目录**

`GenerateJobCA` 使用 `rsa.GenerateKey(rand.Reader, 3072)` 和自签名 `x509.CreateCertificate`。证书约束固定为 `IsCA=true`、`KeyUsageCertSign|CRLSign|DigitalSignature`、有效期 24 小时、随机 128 位序列号。类型：

```go
type JobCA struct { CertPEM []byte; KeyPEM []byte; Certificate *x509.Certificate; SHA256Fingerprint string }
func GenerateJobCA(jobID string, now time.Time) (*JobCA, error)
func (c *JobCA) WriteSecrets(secretsDir string) (certPath string, keyPath string, err error)
func (c *JobCA) NewSunny() (*SunnyNet.Sunny, error)
```

运行时只允许把 `store.SecretsDir()` 的返回值传给 `WriteSecrets`；该方法再次验证目标目录绝对路径，使用权限 `0600` 写入并且不输出路径。`NewSunny` 创建 SunnyNet Certificate manager，调用 `LoadX509Certificate` 加载内存 PEM，再调用 `sunny.SetCert(managerID).SetLoopbackOnly()`；不使用静态证书或 `app.Run()`。

- [ ] **Step 3: 实现只操作 CurrentUser Root 的证书适配器**

命令执行接口固定为：

```go
type CommandRunner interface { Run(context.Context, string, ...string) ([]byte, error) }
type CertificateStore interface { Install(context.Context, string, string) error; RemoveBySHA256(context.Context, string) error; ContainsSHA256(context.Context, string) (bool, error) }
```

Windows 实现只调用 `powershell.exe -NoProfile -NonInteractive -Command`。安装脚本固定使用 `Import-Certificate -CertStoreLocation Cert:\CurrentUser\Root`；禁止 LocalMachine fallback。查找和删除时枚举 CurrentUser Root，计算每张证书 RawData 的 SHA-256，仅当恰好一张等于目标值时使用该对象的 `PSPath` 删除。命令输出只解析布尔值，不写日志。

非 Windows 实现对三个方法统一返回 `POC runtime requires Windows`。

- [ ] **Step 4: 实现只读预检报告**

```go
type PreflightReport struct {
    Passed bool `json:"passed"`; IsolatedVM bool `json:"isolated_vm"`; GitIgnored bool `json:"git_ignored"`
    ExecutableNameOK bool `json:"executable_name_ok"`; LoopbackPortsFree bool `json:"loopback_ports_free"`
    SunnyDriverAbsent bool `json:"sunny_driver_absent"`; DynamicPortBaselineHash string `json:"dynamic_port_baseline_hash"`
    CertificateBaselineHash string `json:"certificate_baseline_hash"`; ListenerBaselineHash string `json:"listener_baseline_hash"`
    ReasonCodes []string `json:"reason_codes"`
}
type VMDetector interface { Detect(context.Context) (bool, string, error) }
type Preflight struct { runner CommandRunner; detector VMDetector }
func (p *Preflight) Run(context.Context, Options) (PreflightReport, error)
```

Windows VM detector 使用 CIM 的 `Win32_ComputerSystem.Manufacturer,Model`，只接受 Hyper-V、VMware、VirtualBox、KVM/QEMU 明确信号。预检只运行读取命令：`git status --porcelain`、`git check-ignore`、`netsh int ipv4 show dynamicport tcp`、`netsh int ipv6 show dynamicport tcp`、CurrentUser Root 枚举、`Get-NetTCPConnection` 和 `Get-Service SunnyFilter2`。输出先规范化再 SHA-256，不保存完整系统清单。

- [ ] **Step 5: 为秘密目录设置当前用户 SID ACL**

Windows 创建 `.poc-secrets/<jobID>` 后使用当前进程 Token 取得 SID，再运行参数化 `icacls.exe <dir> /inheritance:r /grant:r <SID>:(OI)(CI)F`。任何 ACL 命令失败都中止，且不得继续生成 CA。测试 fake runner 必须断言参数中没有用户名拼接或 shell 字符串执行。

- [ ] **Step 6: 运行 CA/预检测试并提交**

Run: `go test ./internal/poc -run 'TestGenerateJobCA|TestPreflight|TestCertificate|TestSecretACL' -count=1`

Expected: PASS；测试不接触真实证书库和驱动。

```powershell
git add internal/poc/ca.go internal/poc/ca_test.go internal/poc/preflight.go internal/poc/preflight_test.go internal/poc/certstore_windows.go internal/poc/certstore_other.go
git commit -m "feat: add isolated CA and POC preflight"
```

## Task 9：实现最小 HTML 注入和回环进程代理包装

**Files:**

- Create: `internal/poc/injector.go`
- Create: `internal/poc/injector_test.go`
- Create: `internal/poc/proxy.go`
- Create: `internal/poc/proxy_test.go`

- [ ] **Step 1: 写纯函数注入和代理调用顺序红灯测试**

```go
func TestInjectHTMLOnlyOnApprovedChannelsPages(t *testing.T) {
    in := []byte("<html><head></head><body></body></html>")
    got, changed := InjectHTML("channels.weixin.qq.com", "/web/pages/home", in, []byte("window.poc=true"), BridgeConfig{Port:2026, Token:"secret"})
    if !changed || !bytes.Contains(got, []byte("window.__WX_CHANNEL_POC_CONFIG__")) { t.Fatalf("not injected: %s", got) }
    if _, changed := InjectHTML("evil.example", "/web/pages/home", in, nil, BridgeConfig{}); changed { t.Fatal("foreign host injected") }
    if _, changed := InjectHTML("channels.weixin.qq.com", "/unapproved", in, nil, BridgeConfig{}); changed { t.Fatal("unapproved path injected") }
}

func TestProxyStartsLoopbackThenExactProcessRule(t *testing.T) {
    sunny := &fakeSunny{}
    proxy := NewProxy(sunny, "127.0.0.1:2025")
    if err := proxy.StartListener(); err != nil { t.Fatal(err) }
    if err := proxy.StartProcessRule(); err != nil { t.Fatal(err) }
    if diff := cmp.Diff([]string{"configure-loopback:2025", "start-listener", "add-name:WeChatAppEx.exe", "start-process"}, sunny.calls); diff != "" { t.Fatal(diff) }
}
```

Run: `go test ./internal/poc -run 'TestInjectHTML|TestProxyStarts' -v`

Expected: FAIL。

- [ ] **Step 2: 实现无日志、无 JS bundle 改写的注入器**

允许集合固定为：

```go
var approvedPagePaths = map[string]struct{}{
    "/web/pages/home": {}, "/web/pages/feed": {}, "/web/pages/profile": {},
}
```

`InjectHTML` 只在 host 精确为 `channels.weixin.qq.com`、路径在集合内、Content-Type 为 HTML 且存在首个 `<head>` 时工作。配置通过 `json.Marshal(BridgeConfig)` 生成，插入：

```html
<script>window.__WX_CHANNEL_POC_CONFIG__={"port":2026,"token":"..."};</script>
<script>/* embedded poc_api_client.js */</script>
```

不得记录生成的 HTML、Token、页面 URL 或响应体。

- [ ] **Step 3: 用窄接口包装 SunnyNet**

```go
type sunnyCore interface {
    ConfigureLoopbackPort(int) error
    SetHTTPCallback(func(*SunnyNet.HttpConn))
    StartListener() error
    AddProcessName(string) error
    StartProcess() error
    DeleteProcessName(string) error
    CloseListener() error
    StopProcess(bool) error
}
type realSunnyCore struct { sunny *SunnyNet.Sunny }
type Proxy struct { /* exact address, sunny core, installed-by-job flag, injector */ }
func NewProxy(s sunnyCore, addr string) *Proxy
func (p *Proxy) StartListener() error
func (p *Proxy) StartProcessRule() error
func (p *Proxy) Cleanup(unregisterDriver bool) error
```

`realSunnyCore` 是唯一接触 SunnyNet fluent API 的适配器，把其返回值/布尔值转换为普通 `error`，因此测试 fake 不依赖具体 `*SunnyNet.Sunny`。`StartListener` 拒绝非 loopback，`StartProcessRule` 只允许 `WeChatAppEx.exe`。HTTP callback 只在允许页面请求阶段删除 `Accept-Encoding`，只在允许 HTML 响应阶段调用纯函数注入。它不注册 APIRouter、下载处理器、指标、云、更新或硬件指纹代码。

- [ ] **Step 4: 验证清理逆序和幂等**

测试两次调用 `Cleanup(true)`，断言第一次顺序为 `del-name → close → stop-process:true`，第二次不重复删除非本任务目标且返回 nil。

Run: `go test ./internal/poc -run 'TestInjectHTML|TestProxy' -count=1`

Expected: PASS。

- [ ] **Step 5: 提交代理边界**

```powershell
git add internal/poc/injector.go internal/poc/injector_test.go internal/poc/proxy.go internal/poc/proxy_test.go
git commit -m "feat: add minimal loopback WeChat proxy"
```

## Task 10：实现搜索分页、作品规范化和覆盖状态

**Files:**

- Create: `internal/poc/search.go`
- Create: `internal/poc/search_test.go`
- Create: `internal/poc/testdata/search-page-1.json`
- Create: `internal/poc/testdata/search-page-end.json`

- [ ] **Step 1: 创建不含真实数据的搜索 fixtures**

第一页使用 `data.objectList`，含两个有效作品、一个缺少 ID 的结构异常项和非空 `lastBuffer`。末页使用 `data.data.objectList`，含一个有效非视频作品，并显式包含空字符串 `lastBuffer`。所有账号、ID 和正文使用 `fixture-*` 值。

- [ ] **Step 2: 写顺序、耗尽和缺失 ID 红灯测试**

```go
func TestCollectSearchPreservesOrderAndExhaustion(t *testing.T) {
    api := newFixtureAPI(t, "search-page-1.json", "search-page-end.json")
    works, coverage, err := NewCollector(api, testRecorder(t), testStore(t), testClock()).CollectWorks(context.Background(), DefaultOptions())
    if err != nil { t.Fatal(err) }
    if len(works) != 3 || works[0].Locator.SearchRank != 1 || works[2].Locator.SearchRank != 3 { t.Fatalf("works=%+v", works) }
    if coverage != CoverageSourceExhausted { t.Fatalf("coverage=%s", coverage) }
}

func TestEmptyPageWithoutExplicitLastBufferIsNotExhaustion(t *testing.T)
func TestRepeatedSearchMarkerStopsAsIncomplete(t *testing.T)
func TestSearchStopsAtTenUniqueValidWorks(t *testing.T)
```

Run: `go test ./internal/poc -run 'TestCollectSearch|TestEmptyPage|TestRepeatedSearch|TestSearchStops' -v`

Expected: FAIL。

- [ ] **Step 3: 实现 API、时钟和采集器公共接口**

```go
type PageAPI interface { Call(context.Context, string, any) ([]byte, error) }
type Clock interface { Now() time.Time; Sleep(context.Context, time.Duration) error }
type Collector struct { api PageAPI; evidence *EvidenceRecorder; store *Store; clock Clock; lastRequest time.Time; sequence int }
func NewCollector(PageAPI, *EvidenceRecorder, *Store, Clock) *Collector
func (c *Collector) call(context.Context, string, any) ([]byte, SourceRef, error)
func (c *Collector) CollectWorks(context.Context, Options) ([]Work, CoverageStatus, error)
```

`call` 保证任意两次微信 API 请求开始时间至少相隔一秒，并在 JSON 解析前调用 `EvidenceRecorder.Observe` 和 `Store.WriteEvidence`。

- [ ] **Step 4: 实现已知搜索结构解析**

使用 `json.Decoder.UseNumber`，只接受 `objectList` 位于顶层 `data.objectList` 或 `data.data.objectList`。结束条件必须是找到同一数据对象中的 `lastBuffer` 字段且其值为显式空字符串；字段缺失、空页、本地错误和重复 marker 都返回 `CoverageIncomplete`。作品映射：

- ID：`id`；缺失时记录结构 reason，不占排名。
- nonce：`objectNonceId`，缺失输出 `null`。
- 标题：`objectDesc.description`。
- 账号：依次读取明确字段 `contact.username`/`username` 和 `contact.nickname`/`nickname`，没有则 `null`。
- 媒体 raw code：`objectDesc.mediaType`；只有代码 `2`、`4` 根据当前上游下载判断明确映射为 `video`，其他代码先映射 `unknown`，不得由标题或 DOM 推断。
- 排名按去重后的有效 ID 从 1 开始，最多 10。

- [ ] **Step 5: 运行搜索测试并提交**

Run: `go test ./internal/poc -run 'TestCollectSearch|TestEmptyPage|TestRepeatedSearch|TestSearchStops' -count=1`

Expected: PASS。

```powershell
git add internal/poc/search.go internal/poc/search_test.go internal/poc/testdata/search-page-1.json internal/poc/testdata/search-page-end.json
git commit -m "feat: collect ordered public search works"
```

## Task 11：实现一级评论、二级回复、上限和来源关系

**Files:**

- Create: `internal/poc/comments.go`
- Create: `internal/poc/comments_test.go`
- Create: `internal/poc/testdata/comments-top.json`
- Create: `internal/poc/testdata/comments-replies.json`

- [ ] **Step 1: 创建评论 fixtures**

`comments-top.json` 必须包含：一个有 ID 的一级文本评论、一个无 ID 的一级媒体评论、一个内嵌二级回复、`countInfo.commentCount`、非空 `lastBuffer`。第二页由测试内联返回空 `lastBuffer`。`comments-replies.json` 包含内嵌回复的重复 ID 和一个新回复，验证去重与父 ID 保留。全部值使用 `fixture-*`，URL 必须包含一个待剥离的查询参数。

- [ ] **Step 2: 写分页、限制、缺失 ID 和父子关系红灯测试**

```go
func TestCollectCommentsMapsTopLevelAndReplies(t *testing.T) {
    opts := DefaultOptions()
    api := newCommentFixtureAPI(t)
    collector := NewCollector(api, testRecorder(t), testStore(t), testClock())
    work := fixtureWork("work-1", "nonce-1", 1)
    got, summary, err := collector.CollectComments(context.Background(), opts, work)
    if err != nil { t.Fatal(err) }
    if summary.TopLevel != 2 || summary.Replies != 2 { t.Fatalf("summary=%+v", summary) }
    assertComment(t, got, "fixture-reply-2", 2, "fixture-top-1", "fixture-top-1")
}

func TestMissingCommentIDIsNotSynthesizedOrCrossPageDeduped(t *testing.T)
func TestTopLevelStopsAt100AndMarksTruncated(t *testing.T)
func TestRepliesStopAt200PerWorkAndMarksTruncated(t *testing.T)
func TestParentAndRootRemainNullWhenSourceOmitsThem(t *testing.T)
func TestIPRegionAndTimePreserveRawSourceSemantics(t *testing.T)
```

Run: `go test ./internal/poc -run 'TestCollectComments|TestMissingComment|TestTopLevelStops|TestRepliesStop|TestParentAndRoot|TestIPRegion' -v`

Expected: FAIL。

- [ ] **Step 3: 实现评论响应解析和精确分页条件**

类型和方法：

```go
type CommentSummary struct { TopLevel int; Replies int; Truncated bool; Reasons []string; Partial bool }
func (c *Collector) CollectComments(context.Context, Options, Work) ([]Comment, CommentSummary, error)
func parseCommentPage(raw []byte) (items []map[string]any, lastBuffer string, lastBufferPresent bool, reported int, err error)
func mapComment(item map[string]any, level int, workID *string, retrievalRoot *string, source SourceRef) (Comment, []FieldResult)
```

解析只接受已知 `data.commentInfo`、`data.lastBuffer`、`data.countInfo.commentCount`。一级分页使用作品 ID+nonce；回复分页使用作品 ID+根评论 ID。每页写证据并保存检查点后才能请求下一页。

- [ ] **Step 4: 实现 ID 去重和缺失 ID 语义**

- 有 ID 的一级评论和回复分别按源 ID 去重，保留第一次出现顺序。
- 无 ID 记录每个 `evidence_ref + source.ordinal` 保留一次，不创建 `idx-*` 或其他替代 ID，不参与跨页去重。
- 一级评论的 `parent_comment_id`、`root_comment_id` 为源响应明确的值；`"0"` 规范化为 `null`。
- `retrieval_root_comment_id` 只对独立回复分页设置，不写入 `parent_comment_id`。
- 一级响应的内嵌回复先加入，独立回复分页再按 ID 合并。
- 无 ID 一级评论报告有回复时不请求回复分页，作品 `Partial=true`，reason=`missing_root_comment_id`。

- [ ] **Step 5: 实现字段映射和安全规范化**

- 正文：`content`；秘密命中时 `null/redacted_for_safety`。
- 内容 raw type：`contentType`；没有经 fixture 证明的代码一律 `unknown`。
- 账号：`username`、`nickname`、`headUrl`；头像通过 `SafeURL`。
- 时间：保留 `createtime` 字符串；正 Unix 秒生成 UTC RFC3339，否则规范化字段为 `null/invalid_format`。
- IP 属地：先读 `ipRegionInfo.regionText`，再读 `ipRegion`，不解析真实 IP。

- [ ] **Step 6: 实现上限**

一级评论达到 100 后停止该作品的一级分页。回复在该作品内累计达到 200 后停止所有后续根评论回复分页。两者均在 `Work.Truncation` 中加入 `top_level_limit` 或 `reply_limit`；达到上限不是错误。

- [ ] **Step 7: 运行评论测试并提交**

Run: `go test ./internal/poc -run 'TestCollectComments|TestMissingComment|TestTopLevelStops|TestRepliesStop|TestParentAndRoot|TestIPRegion' -count=1`

Expected: PASS。

```powershell
git add internal/poc/comments.go internal/poc/comments_test.go internal/poc/testdata/comments-top.json internal/poc/testdata/comments-replies.json
git commit -m "feat: collect bounded comment threads"
```

## Task 12：实现人工等待、检查点恢复和三维结论

**Files:**

- Create: `internal/poc/wait.go`
- Create: `internal/poc/wait_test.go`
- Create: `internal/poc/status.go`
- Create: `internal/poc/status_test.go`
- Modify: `internal/poc/search.go`
- Modify: `internal/poc/comments.go`

- [ ] **Step 1: 写 300 秒、单次延长和状态红灯测试**

```go
func TestWaitAllowsOne300SecondExtension(t *testing.T) {
    clock := newManualClock()
    controls := make(chan OperatorCommand, 2)
    ready := make(chan struct{})
    waiter := NewWaitController(clock, HumanWaitPolicy{Timeout:300*time.Second, Extension:300*time.Second, MaxExtensions:1}, controls)
    done := make(chan WaitResult, 1)
    go func(){ done <- waiter.Wait(context.Background(), WaitLogin, 0, ready) }()
    clock.Advance(299*time.Second); controls <- OperatorExtend
    clock.Advance(300*time.Second)
    if got := <-done; got != WaitTimedOut { t.Fatalf("got=%s", got) }
}

func TestSecondExtensionIsRejected(t *testing.T)
func TestTimeoutBeforeWorksRequiresHuman(t *testing.T)
func TestTimeoutAfterWorkIsPartial(t *testing.T)
func TestThreeDimensionalStatusForThreeExhaustedWorks(t *testing.T)
func TestNoRepliesIsCapabilityInconclusiveNotCoverageFailure(t *testing.T)
func TestReadySignalResolvesWaitWithoutRequest(t *testing.T)
func TestTransientRetryUsesTwoAndFiveSecondBackoff(t *testing.T)
func TestRateLimitAndAccessDeniedDoNotRetry(t *testing.T)
```

Run: `go test ./internal/poc -run 'TestWait|TestSecondExtension|TestReadySignal|TestTransientRetry|TestRateLimit|TestTimeout|TestThreeDimensional|TestNoReplies' -v`

Expected: FAIL。

- [ ] **Step 2: 实现人工等待控制器**

```go
type WaitReason string
const (WaitLogin WaitReason = "login"; WaitVerification WaitReason = "verification"; WaitTargetContext WaitReason = "target_context")
type OperatorCommand string
const (OperatorExtend OperatorCommand = "extend"; OperatorCancel OperatorCommand = "cancel")
type WaitResult string
const (WaitResolved WaitResult = "resolved"; WaitTimedOut WaitResult = "timed_out"; WaitCancelled WaitResult = "cancelled")
type WaitController struct { clock Clock; policy HumanWaitPolicy; commands <-chan OperatorCommand }
func NewWaitController(Clock, HumanWaitPolicy, <-chan OperatorCommand) *WaitController
func (w *WaitController) Wait(context.Context, WaitReason, int, <-chan struct{}) WaitResult
```

最后一个参数是只接收的就绪信号；页面 bridge 证明所需方法和目标上下文恢复时关闭该 channel，`Wait` 返回 `WaitResolved`。每次 `Wait` 独立记录延长次数；收到一次 `extend` 后重置为固定 300 秒，第二次返回拒绝事件但不重置计时。操作提示只含 reason 和作品排名，不含作品 ID、账号或正文。

- [ ] **Step 3: 将页面上下文错误接入等待**

统一错误分类器只根据错误码和固定文本 `-70003`、`JSAPI_JSONPARSE_FAILED` 返回 `WaitTargetContext`。等待期间不发送微信请求；页面桥 state 重新满足方法和目标上下文后通过就绪 channel 返回 `WaitResolved`，并仅重试当前只读请求一次。超时/取消先 `SaveCheckpoint`，再返回带进度的终止原因。

普通瞬时网络错误使用固定 `RetryPolicy{MaxRetries:2, Backoff:[]time.Duration{2*time.Second, 5*time.Second}}`；每次重试仍必须满足全局 1 秒请求间隔。限流、访问拒绝、方法缺失、安全扫描失败和结构错误不自动重试，立即保存检查点并产生固定错误类别。重试日志只记录序号、等待时长和固定错误类别，不记录原始错误字符串。

- [ ] **Step 4: 实现状态判定**

```go
type OutcomeInput struct {
    SearchComplete bool; SourceExhausted bool; ValidWorks int; CompletedWorks int
    TopLevelComments int; Replies int; RequiredFieldStatuses map[string]FieldStatus
    HumanTimedOut bool; HumanCancelled bool; SafetyFailed bool; CleanupFailed bool; SchemaFailed bool
}
func EvaluateOutcome(OutcomeInput) (JobStatus, CapabilityStatus, CoverageStatus, []string)
```

判定顺序只在各维度内部使用：安全/清理失败强制 `JobFailed+CapabilityFailed`；否则运行未开始为 `JobRequiresHuman`、有进度未完成为 `JobPartial`、完整为 `JobCompleted`。覆盖只看 10 个作品或明确耗尽。能力在运行完整且有一级+二级回复时为 verified；只有 `missing_in_source` 缺口时为 verified_with_gaps；无回复、人工终止、结构错误或待验证字段仅被安全脱敏时为 inconclusive。

- [ ] **Step 5: 验证恢复从最后完整页继续**

测试先保存第二页完成 checkpoint，重建 Collector 后断言下一请求 marker 是第三页 marker，已保存有 ID 记录不重复，无 ID 记录只按其原 evidence+ordinal 恢复。

Run: `go test ./internal/poc -run 'TestWait|TestReadySignal|TestTransientRetry|TestRateLimit|TestOutcome|TestResume|TestTimeout|TestThreeDimensional|TestNoReplies' -count=1`

Expected: PASS。

- [ ] **Step 6: 提交等待与结论**

```powershell
git add internal/poc/wait.go internal/poc/wait_test.go internal/poc/status.go internal/poc/status_test.go internal/poc/search.go internal/poc/comments.go
git commit -m "feat: add bounded human waits and POC outcomes"
```

## Task 13：组装生命周期、CLI 和幂等清理

**Files:**

- Create: `internal/poc/runtime.go`
- Create: `internal/poc/runtime_windows.go`
- Create: `internal/poc/runtime_other.go`
- Create: `internal/poc/runtime_test.go`
- Create: `internal/poc/app.go`
- Create: `internal/poc/app_test.go`
- Create: `cmd/wx_channel_poc/main.go`
- Modify: `internal/poc/log.go`
- Create: `internal/poc/log_test.go`

- [ ] **Step 1: 写资源创建/清理顺序红灯测试**

```go
func TestRuntimeCleansUpInReverseOrderOnCollectorFailure(t *testing.T) {
    deps, events := fakeRuntimeDeps(errors.New("collector failed"))
    err := NewRuntime(deps).Run(context.Background(), approvedTestOptions())
    if err == nil { t.Fatal("expected error") }
    want := []string{"preflight", "create-ca", "install-ca", "start-bridge", "start-proxy", "start-driver", "collect", "stop-requests", "remove-process-rule", "stop-proxy", "stop-bridge", "remove-ca", "destroy-secrets", "write-cleanup-receipt"}
    if diff := cmp.Diff(want, *events); diff != "" { t.Fatal(diff) }
}

func TestRunRefusesWithoutAckOrInteractiveApply(t *testing.T)
func TestCleanupTwiceIsIdempotent(t *testing.T)
func TestSignalUsesSameCleanupPath(t *testing.T)
```

Run: `go test ./internal/poc -run 'TestRuntime|TestRunRefuses|TestCleanupTwice|TestSignal' -v`

Expected: FAIL。

- [ ] **Step 2: 定义可测试的运行依赖和精确生命周期**

```go
type RuntimeDeps struct {
    Preflight func(context.Context, Options) (PreflightReport, error)
    CreateCA func(string) (*JobCA, error)
    CertStore CertificateStore
    BridgeStart func(context.Context, string, string) (Bridge, io.Closer, error)
    ProxyFactory func(context.Context, *JobCA, string, string) (RuntimeProxy, error)
    Collect func(context.Context, Bridge, *Store, Options) (Dataset, Validation, error)
    StoreFactory func(Options, string) (*Store, error)
    Logger SafeLogger
}
type Runtime struct { deps RuntimeDeps }
type RuntimeProxy interface {
    StartListener() error
    StartProcessRule() error
    Cleanup(unregisterDriver bool) error
}
func NewRuntime(RuntimeDeps) *Runtime
func (r *Runtime) Run(context.Context, Options) error
func (r *Runtime) Cleanup(context.Context, string) error
```

真实顺序固定为：预检 → 生成 job ID/目录 → 生成 CA/Token → 显示不敏感变更摘要并要求操作员输入精确文本 `APPLY` → CurrentUser 安装 CA → 启动 `127.0.0.1` bridge → 启动回环代理 → 添加并启动 `WeChatAppEx.exe` 驱动规则 → 等待页面能力 → 串行采集 → 写 dataset/validation/manifest → 逆序清理 → 写 cleanup receipt → 最终原子更新状态。

- [ ] **Step 3: 实现 Windows 与非 Windows 适配**

`runtime_windows.go` 使用 build tag `windows` 并连接真实 CertificateStore、Sunny Proxy 和 ACL。它只在预检证明 `SunnyFilter2` 基线不存在时设置 `driverInstalledByJob=true`；清理时只在该标志为 true 时注销驱动。

`runtime_other.go` 使用 `!windows`，`run` 和 `cleanup` 返回 `POC runtime requires isolated Windows VM`；模型、解析、证据和模拟测试仍可跨平台运行。

- [ ] **Step 4: 实现 CLI，不导入旧 `cmd` 或 `internal/app`**

`cmd/wx_channel_poc/main.go` 只使用标准 `flag`：

```go
func main() {
    if len(os.Args) < 2 { fatal("usage: wx_channel_poc <preflight|run|cleanup>") }
    options := poc.DefaultOptions()
    switch os.Args[1] {
    case "preflight": os.Exit(poc.RunPreflightCLI(context.Background(), os.Stdout, options))
    case "run": os.Exit(poc.RunCLI(context.Background(), os.Stdin, os.Stdout, os.Args[2:], options))
    case "cleanup": os.Exit(poc.RunCleanupCLI(context.Background(), os.Stdout, os.Args[2:], options))
    default: fatal("unknown command")
    }
}
```

`run` 唯一授权开关是 `--ack-isolated-vm`；可选 `--allow-encrypted-raw` 默认 false。`cleanup` 必须要求 `--job-id`，并把解析后的目录限制在 `.poc-runtime/<job-id>` 与 `.poc-secrets/<job-id>`。

- [ ] **Step 5: 实现安全日志**

在 `internal/poc/log.go` 中增加 `FileSafeLogger`。它只接受固定事件名和白名单字段：阶段、请求序号、方法、耗时、字节数、错误类别、作品排名、累计数量、截断标记。任何未知字段名使日志调用返回错误；日志写入 `.poc-data/<job-id>/run.log` 前执行 `ScanOrdinaryOutput`。不得记录 ID、正文、账号、URL、Token、证书路径或原始错误字符串。`log_test.go` 必须证明未知事件、未知字段、URL/正文/凭证形态值在创建或追加日志前被拒绝。

- [ ] **Step 6: 运行生命周期测试并提交**

Run: `go test ./internal/poc -run 'TestRuntime|TestRunRefuses|TestCleanupTwice|TestSignal|TestSafeLogger' -count=1`

Expected: PASS；fake 适配器证明所有失败路径执行清理。

```powershell
git add internal/poc/runtime.go internal/poc/runtime_windows.go internal/poc/runtime_other.go internal/poc/runtime_test.go internal/poc/app.go internal/poc/app_test.go internal/poc/log.go internal/poc/log_test.go cmd/wx_channel_poc/main.go
git commit -m "feat: assemble restricted POC runtime"
```

## Task 14：添加来源可验证的构建和安全审计脚本

**Files:**

- Create: `scripts/build-poc.ps1`
- Create: `scripts/poc-security-audit.ps1`
- Create: `scripts/poc-cleanup.ps1`
- Create: `scripts/poc_scripts_test.go`
- Modify: `.gitignore`

- [ ] **Step 1: 写脚本文本安全测试**

`scripts/poc_scripts_test.go` 读取三个脚本并断言：构建目标精确为 `./cmd/wx_channel_poc`，输出精确为 `.poc-build/wx_channel_poc.exe`；不存在 `wx_channel.exe` 执行、`start-insight-data-plane`、`go run .`、通配证书删除、`Remove-Item -Recurse` 对未验证变量；cleanup 必须先调用 `Resolve-Path` 并检查前缀。

Run: `go test ./scripts -run TestPOCScripts -v`

Expected: FAIL，因为脚本尚不存在。

- [ ] **Step 2: 实现只构建不运行的脚本**

`scripts/build-poc.ps1` 固定执行：

```powershell
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$buildRoot = Join-Path $repoRoot '.poc-build'
New-Item -ItemType Directory -Force -Path $buildRoot | Out-Null
Push-Location $repoRoot
try {
    go mod verify
    if ($LASTEXITCODE -ne 0) { throw 'go mod verify failed' }
$env:CGO_ENABLED = '1'
$portableToolBin = Join-Path $repoRoot '.poc-tools\tdm-gcc-10.3.0-2\bin'
if (Test-Path -LiteralPath (Join-Path $portableToolBin 'gcc.exe')) {
    $env:PATH = "$portableToolBin;$env:PATH"
    $env:CC = Join-Path $portableToolBin 'gcc.exe'
    $env:CXX = Join-Path $portableToolBin 'g++.exe'
}
if ((& $env:CC -dumpfullversion).Trim() -ne '10.3.0') { throw 'approved TDM-GCC 10.3.0 is required' }
go build -trimpath -o (Join-Path $buildRoot 'wx_channel_poc.exe') ./cmd/wx_channel_poc
    if ($LASTEXITCODE -ne 0) { throw 'POC build failed' }
    Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $buildRoot 'wx_channel_poc.exe') | ConvertTo-Json -Compress
}
finally { Pop-Location }
```

它不得删除或覆盖仓库跟踪的任何 exe。

- [ ] **Step 3: 实现静态和二进制安全审计**

`poc-security-audit.ps1` 依次：验证编译器精确为 TDM-GCC 10.3.0；运行 `go test ./internal/pocaudit`；运行嵌套 SunnyNet 测试；重新计算两个 NFAPI DLL 和四个驱动的 SHA-256，并验证四个驱动 Authenticode 有效、两个 DLL 的未签名状态与 `PROVENANCE.md` 一致；用 `go list -deps ./cmd/wx_channel_poc` 拒绝 `internal/cloud`、`internal/metrics`、`internal/database`、`internal/services`、旧 `internal/app`、`github.com/GopeedLab/gopeed`、`github.com/mattn/go-sqlite3` 和 `github.com/go-llsqlite/crawshaw`；用 `rg` 拒绝 POC JS 中写方法和日志；把构建产物按 ASCII 读取并拒绝 PEM 私钥标记、默认云 Hub、Cloudflare、GitHub update URL；用 `go version -m` 和 `Get-FileHash` 生成不含秘密的 `.poc-build/provenance.json`。

- [ ] **Step 4: 实现精确 cleanup 包装脚本**

`scripts/poc-cleanup.ps1` 的 `-JobId` 参数只接受正则 `^[a-zA-Z0-9-]{1,80}$`，解析 `.poc-build/wx_channel_poc.exe`，执行其 `cleanup --job-id $JobId`，再验证对应任务的 runtime 与 secrets 目录解析路径均以仓库内相应根目录开头并且不含 reparse point。脚本本身不直接删除证书、驱动或目录。

- [ ] **Step 5: 运行脚本测试并提交**

Run:

```powershell
go test ./scripts -run TestPOCScripts -count=1
git check-ignore .poc-build/probe .poc-secrets/probe .poc-data/probe .poc-runtime/probe var/probe
```

Expected: PASS，六个 probe 均输出为 ignored。

```powershell
git add .gitignore scripts/build-poc.ps1 scripts/poc-security-audit.ps1 scripts/poc-cleanup.ps1 scripts/poc_scripts_test.go
git commit -m "build: add auditable POC build and cleanup scripts"
```

## Task 15：完成无驱动模拟集成、POC 范围测试和源码构建

**Files:**

- Create: `internal/poc/integration_test.go`
- Modify: `internal/poc/testdata/*.json`（仅在测试暴露结构不一致时修正 fixture）

- [ ] **Step 1: 写模拟页面客户端端到端测试**

`integration_test.go` 启动真实 `BridgeServer` 和 Go 编写的模拟 WebSocket 页面客户端；客户端声明三个只读方法，并按请求序号返回 fixtures。测试不得启动 SunnyNet Proxy、证书库或驱动。

```go
func TestSimulatedPOCWritesSafeStructuredOutput(t *testing.T) {
    env := newSimulatedEnvironment(t)
    result := env.Run(DefaultOptions())
    if result.Job.Status != JobCompleted { t.Fatalf("status=%s", result.Job.Status) }
    if result.Job.CapabilityStatus != CapabilityVerified { t.Fatalf("capability=%s", result.Job.CapabilityStatus) }
    if result.Job.CoverageStatus != CoverageSourceExhausted { t.Fatalf("coverage=%s", result.Job.CoverageStatus) }
    if len(result.Works) != 3 { t.Fatalf("works=%d", len(result.Works)) }
    assertNoFileMatches(t, env.JobDir, []byte("fixture-raw-token"))
    assertNoFiles(t, filepath.Join(env.JobDir, "raw-evidence"), "*.json", "*.enc")
}

func TestSimulatedPOCEnforces100And200Caps(t *testing.T)
func TestSimulatedPOCContextWaitTimeoutCleansUp(t *testing.T)
func TestSimulatedPOCOutputReferencesEveryEvidenceHash(t *testing.T)
func TestSimulatedPOCOrdinaryFilesPassSecretScanner(t *testing.T)
```

- [ ] **Step 2: 运行模拟集成测试**

Run: `go test ./internal/poc -run TestSimulatedPOC -count=1 -v`

Expected: PASS；测试输出只位于 `t.TempDir()`。

- [ ] **Step 3: 运行根模块 POC 范围测试**

此时 Task 2 已移除导入副作用，才允许执行 POC 相关包：

```powershell
go test ./internal/poc ./internal/pocaudit ./scripts -count=1
```

Expected: PASS；不得出现 `netsh`、证书安装、Cloud Hub、更新检查或硬件指纹输出。不得以 `go test ./...` 作为 POC 门禁，因为上游旧应用在 Windows CGO 下同时链接两套 SQLite amalgamation；安全审计改为证明 POC 依赖图不含这两套旧依赖。

- [ ] **Step 4: 运行 SunnyNet 嵌套模块测试和 Node 测试**

```powershell
go test github.com/qtgolang/SunnyNet/SunnyNet github.com/qtgolang/SunnyNet/src/nfapi github.com/qtgolang/SunnyNet/src/CrossCompiled -count=1
node internal/pocassets/poc_api_client.test.js
```

Expected: 全部 PASS。

- [ ] **Step 5: 从源码构建但不运行 POC**

Run:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/build-poc.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/poc-security-audit.ps1
```

Expected: 生成 `.poc-build/wx_channel_poc.exe` 和 `.poc-build/provenance.json`；安全审计 PASS。不得执行生成的 exe。

- [ ] **Step 6: 检查工作树和提交模拟集成**

```powershell
git status --short
git diff --check
git add internal/poc/integration_test.go internal/poc/testdata
git commit -m "test: verify restricted POC end to end"
```

Expected: 提交后只剩 Git 忽略的 `.poc-build/` 本地产物，`git status --short` 为空。

## Task 16：编写隔离虚拟机运行手册和最终开发验收

**Files:**

- Create: `docs/runbooks/wechat-channel-comment-poc.md`
- Modify: `README.md`（只增加指向安全运行手册的链接，不加入真实结果）

- [ ] **Step 1: 写运行手册的固定安全顺序**

运行手册必须包含以下有序阶段：

1. 创建可销毁 Windows 虚拟机，不挂载宿主机微信数据目录。
2. 在虚拟机中安装 PC 微信并人工登录。
3. 确认视频号可访问，记录无 POC CA/驱动的基线。
4. 创建名为 `wechat-login-baseline` 的快照。
5. 从受控 fork 的 `codex/wechat-channel-comment-poc` 分支检出源码。
6. 准备官方 TDM-GCC 10.3.0（核对固定 SHA-256；仅装在 VM 或解压到 `.poc-tools/`），运行 `scripts/build-poc.ps1` 和 `scripts/poc-security-audit.ps1`，不运行仓库跟踪 exe。
7. 运行 `.poc-build\wx_channel_poc.exe preflight`；任何失败均停止。
8. 仅在用户再次确认真实运行检查点后执行 `run --ack-isolated-vm`，并在提示时人工输入 `APPLY`。
9. 登录/真人验证/目标页面等待时只人工操作微信；不使用验证码识别或 UI 自动化。
10. 验证运行自动清理、复制获准保留的普通结果、确认无加密原始证据后回滚快照或销毁虚拟机。

- [ ] **Step 2: 写不泄露数据的验收命令**

手册只打印状态和数量，不打印 dataset 内容：

```powershell
$jobDir = Get-ChildItem -LiteralPath '.poc-data' -Directory | Sort-Object LastWriteTime -Descending | Select-Object -First 1
$manifest = Get-Content -LiteralPath (Join-Path $jobDir.FullName 'manifest.json') -Raw | ConvertFrom-Json
$validation = Get-Content -LiteralPath (Join-Path $jobDir.FullName 'validation.json') -Raw | ConvertFrom-Json
[pscustomobject]@{
    job_status = $manifest.status
    capability_status = $manifest.capability_status
    coverage_status = $manifest.coverage_status
    works = $manifest.counts.works
    top_level_comments = $manifest.counts.top_level_comments
    replies = $manifest.counts.replies
    cleanup_success = $manifest.cleanup_success
    field_reason_codes = @($validation.reason_codes)
} | ConvertTo-Json -Depth 4
```

输出中不得包含作品 ID、账号、昵称、评论正文、URL、Token 或证书路径。

- [ ] **Step 3: 写清理核验和保留规则**

手册要求确认 `cleanup-receipt.json` 中代理、bridge、进程规则、驱动、CA、秘密和临时加密证据均为成功；随后执行一次幂等 cleanup：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/poc-cleanup.ps1 -JobId $manifest.job_id
```

复制到受控位置的文件只允许：`dataset.json`、`validation.json`、`manifest.json`、`evidence/*.json`、`cleanup-receipt.json` 和经过扫描的 `run.log`。保留目录必须在 Git 外或被 Git 忽略，最多保留 7 天。`.enc`、CA、Token、Cookie、登录状态和原始响应不得复制。

- [ ] **Step 4: 运行最终开发验收**

```powershell
go test ./internal/poc ./internal/pocaudit ./scripts -count=1
go test github.com/qtgolang/SunnyNet/SunnyNet github.com/qtgolang/SunnyNet/src/nfapi github.com/qtgolang/SunnyNet/src/CrossCompiled -count=1
node internal/pocassets/poc_api_client.test.js
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/build-poc.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/poc-security-audit.ps1
git diff --check
git status --short
```

Expected: 全部 PASS；Git 工作树只有待提交的运行手册和 README 链接，POC 构建和结果目录被忽略。

- [ ] **Step 5: 提交运行手册**

```powershell
git add docs/runbooks/wechat-channel-comment-poc.md README.md
git commit -m "docs: add isolated POC validation runbook"
```

## Task 17：真实验证检查点（必须再次获得用户确认）

**Files:** 只在 Git 忽略的 `.poc-data/`、`.poc-runtime/`、`.poc-secrets/` 和 `.poc-build/` 中产生本地文件；不得提交真实结果。

- [ ] **Step 1: 暂停并向用户报告开发证据**

报告以下不敏感信息：实现分支、提交列表、POC 范围测试结果、SunnyNet 嵌套模块测试结果、Node 测试结果、构建 SHA-256、安全审计结论、待使用的隔离虚拟机类型。明确说明尚未安装 CA、加载驱动或运行真实采集。

Expected: 用户明确回复允许进入真实虚拟机验证；没有确认时不继续。

- [ ] **Step 2: 在隔离虚拟机内按运行手册执行真实验证**

只执行从当前源码构建的 `.poc-build\wx_channel_poc.exe`。关键词固定“青云装饰”，作品上限 10，每作品一级评论上限 100、回复上限 200。任何登录或真人验证只等待人工处理，单事件 300 秒且最多延长一次。

- [ ] **Step 3: 验证结构与三维结论，不把真实值输出到终端**

运行 Task 16 的状态/数量命令；逐字段验证报告应覆盖 comment ID、parent ID、正文/非文本类型、账号、时间、IP 属地、作品媒体类型和定位字段。少于 10 个但明确搜索耗尽时，`coverage_status=source_exhausted_below_target`，能力状态独立判断。

- [ ] **Step 4: 清理并销毁或回滚虚拟机**

确认自动清理回执，运行幂等 cleanup，确认 `.poc-secrets/<job-id>` 和任何 `.enc` 已删除。把获准普通结果复制到受控保留位置后，回滚到 `wechat-login-baseline` 或销毁 VM；若驱动或证书残留无法在运行系统内确认移除，必须销毁 VM，并把 `cleanup_success` 设为 false 直到销毁完成。

- [ ] **Step 5: 生成不含真实数据的能力摘要**

对用户只报告：三个状态、作品/一级评论/回复数量、字段 present/missing/invalid/redacted 计数、安全检查和清理是否成功。不得粘贴作品 ID、账号、正文、IP 属地文本、URL、Token 或证书信息。

## 完成定义

- 所有提交都位于 `codex/wechat-channel-comment-poc`，本地 `main` 仍跟踪 `upstream/main`。
- 源码和 POC 二进制没有 PEM 私钥，导入 SunnyNet 不执行 `netsh`。
- POC 依赖图不包含旧 App、Cloud、Metrics、Radar、Database、Download 或 Update 服务。
- 代理、HTTP/WebSocket 只监听 `127.0.0.1`，WebSocket Token 只出现在 `Sec-WebSocket-Protocol` 请求头和页面内存，不进入 URL 或日志。
- 只有 `finderSearch`、`finderGetCommentDetail`、`finderGetCommentList` 能跨页面桥调用。
- 普通输出通过秘密扫描，原始响应默认不落盘，可选密文在任务结束立即删除。
- 人工等待按 300 秒 + 一次 300 秒延长执行，超时保存检查点并清理。
- 少于 10 个且明确耗尽时覆盖状态与能力状态独立。
- 真实运行只发生在获批的可回滚/可销毁 Windows 虚拟机中，最终完成精确清理和 VM 回滚/销毁。
