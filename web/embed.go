// Package webui 通过 go:embed 打包 Vite 构建产物 dist/。
// 开发时使用 `npm run dev`（Vite dev server 代理 /api 到 Go 服务）；
// 发布前在 web/ 目录执行 `npm run build` 生成 dist/，再 go build 即可得到单二进制。
package webui

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed dist
var embedded embed.FS

// DistFS 返回可供 http.FileServer 使用的前端资源 FS（根目录即 dist）。
// 当 dist 中只有占位文件、尚未真正构建前端时，第二个返回值为 false。
func DistFS() (fs.FS, bool) {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, false
	}
	b, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return sub, false
	}
	// 占位 index.html 带该标记，用于区分「未构建」。
	if len(b) < 400 && strings.Contains(string(b), "story-web-placeholder") {
		return sub, false
	}
	return sub, true
}
