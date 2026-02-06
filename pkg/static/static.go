//go:build embed_frontend
// +build embed_frontend

package static

import (
	"embed"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed dist/*
var staticFiles embed.FS

// GetFileSystem 返回嵌入的静态文件系统
func GetFileSystem() http.FileSystem {
	// 从 embed.FS 中提取 dist 目录
	fsys, err := fs.Sub(staticFiles, "dist")
	if err != nil {
		// 如果提取失败，返回原始文件系统
		return http.FS(staticFiles)
	}
	return http.FS(fsys)
}

// ServeStaticFiles 提供静态文件服务的中间件
// 如果请求的是 API 路径或 WebSocket 路径，则跳过
// 否则尝试提供静态文件，如果文件不存在则返回 index.html（支持 React Router）
func ServeStaticFiles() gin.HandlerFunc {
	fsys := GetFileSystem()
	fileServer := http.StripPrefix("/", http.FileServer(fsys))

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// 注意：在 NoRoute 情况下，这些路径理论上不应该到达这里
		// 但为了安全起见，仍然检查并返回 404
		if strings.HasPrefix(path, "/api") ||
			strings.HasPrefix(path, "/ws") ||
			path == "/metrics" ||
			path == "/health" ||
			strings.HasPrefix(path, "/swagger") {
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}

		// 如果请求的是 /assets/ 目录本身（没有文件名），直接返回 404
		// 避免 http.FileServer 返回 301 重定向
		if path == "/assets/" || path == "/assets" {
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}

		// 尝试打开文件
		filePath := strings.TrimPrefix(path, "/")
		if filePath == "" {
			filePath = "index.html"
		}

		// 检查文件是否存在
		file, err := fsys.Open(filePath)
		if err == nil {
			file.Close()
			// 文件存在，直接提供
			fileServer.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}

		// 文件不存在，检查是否是静态资源（有扩展名）
		ext := filepath.Ext(filePath)
		if ext != "" {
			// 有扩展名但不是文件，返回 404
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}

		// 没有扩展名或文件不存在，尝试返回 index.html（支持 React Router）
		// 先尝试打开 index.html
		indexFile, err := fsys.Open("index.html")
		if err != nil {
			// index.html 也不存在，说明前端文件没有正确嵌入
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}
		indexFile.Close()

		// index.html 存在，返回它（支持 React Router）
		// 创建一个新的请求副本，将路径设置为 index.html
		// 这样不会影响原始的请求路径
		req := c.Request.Clone(c.Request.Context())
		req.URL.Path = "/index.html"
		fileServer.ServeHTTP(c.Writer, req)
		c.Abort()
	}
}

