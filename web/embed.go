// Package web 内嵌 Web 控制台静态资源，避免依赖进程工作目录
package web

import "embed"

//go:embed console.html index.html about.html icon.png css js docs
var FS embed.FS
