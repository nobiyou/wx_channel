# Go System Proxy Windows系统代理配置

[![Documentation](https://godoc.org/github.com/Trisia/gosysproxy?status.svg)](https://pkg.go.dev/github.com/Trisia/gosysproxy)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/Trisia/gosysproxy)

通过系统调用的方式实现Windows系统的代理配置、状态查询。

## Quick Start

获取
```
go get -u github.com/Trisia/gosysproxy
```

API
```go
// SetPAC 设置PAC代理模式
// scriptLoc: 脚本地址，如: "http://127.0.0.1:7777/pac"
func SetPAC(scriptLoc string)

// SetGlobalProxy 设置全局代理
// proxyServer: 代理服务器host:port，例如: "127.0.0.1:7890"
// bypass: 忽略代理列表,这些配置项开头的地址不进行代理
func SetGlobalProxy(proxyServer string, bypasses ...string) error

// Off 关闭代理
func Off() error

// Flush 更新系统配置使生效
func Flush()

// Status 获取当前系统代理配置
func Status() (*ProxyStatus, error)
```

详见： [《Go System Proxy 接口文档》](https://pkg.go.dev/github.com/Trisia/gosysproxy)

## Demo

```go
package main

import (
	"github.com/Trisia/gosysproxy"
	"time"
)

func main() {
    // 设置全局代理
    err := gosysproxy.SetGlobalProxy("127.0.0.1:7890")
    if err{
        panic(err)
    }
    
    time.Sleep(time.Second * 60)
    
    err := gosysproxy.Off()
    if err{
        panic(err)
    }
}
```

完整示例详见： [main_test.go](main_test.go)


## 致谢

- [Noisyfox/sysproxy](https://github.com/Noisyfox/sysproxy)
