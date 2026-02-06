//go:build !embed_frontend
// +build !embed_frontend

package static

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetFileSystem 返回本地文件系统（开发模式）
func GetFileSystem() http.FileSystem {
	// 尝试从多个可能的位置查找前端构建产物
	possiblePaths := []string{
		"ui/web/dist",
		"../ui/web/dist",
		"./dist",
		"dist",
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return http.Dir(path)
		}
	}

	// 如果都找不到，返回一个空的文件系统
	return http.Dir(".")
}

// ServeStaticFiles 提供静态文件服务的中间件（开发模式）
func ServeStaticFiles() gin.HandlerFunc {
	fileServer := http.FileServer(GetFileSystem())

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
		fsys := GetFileSystem()
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

		// 没有扩展名或文件不存在，返回 index.html（支持 React Router）
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}

