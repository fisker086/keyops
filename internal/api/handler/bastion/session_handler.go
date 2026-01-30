package bastion

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fisker/zjump-backend/internal/model"
	bastionService "github.com/fisker/zjump-backend/internal/service/bastion"
	"github.com/fisker/zjump-backend/pkg/logger"
	"github.com/gin-gonic/gin"
)

type SessionHandler struct {
	service *bastionService.SessionService
}

func NewSessionHandler(service *bastionService.SessionService) *SessionHandler {
	return &SessionHandler{service: service}
}

// ValidateToken 验证会话令牌（供 Proxy 调用）
func (h *SessionHandler) ValidateToken(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, model.Error(400, "Missing token"))
		return
	}

	tokenInfo, err := bastionService.ValidateSessionToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, model.Error(401, err.Error()))
		return
	}

	c.JSON(http.StatusOK, model.Success(gin.H{
		"hostId":   tokenInfo.HostID,
		"userId":   tokenInfo.UserID,
		"username": tokenInfo.Username,
	}))
}

func (h *SessionHandler) CreateSession(c *gin.Context) {
	var req struct {
		HostID string `json:"hostId" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.Error(400, err.Error()))
		return
	}

	// 从上下文获取用户ID（由认证中间件设置）
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, model.Error(401, "未找到用户信息"))
		return
	}

	resp, err := h.service.CreateSession(req.HostID, userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.Error(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, model.Success(resp))
}

func (h *SessionHandler) GetLoginRecords(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	hostID := c.Query("hostId")

	// 获取当前用户信息
	userID, _ := c.Get("userID")

	// 所有用户（包括管理员）只能看自己的登录记录
	filterUserID := userID.(string)

	// 查询虚拟机登录记录（login_records 表，包括成功和失败的登录）
	records, total, err := h.service.GetLoginRecordsByUser(page, pageSize, hostID, filterUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.Error(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, model.Success(gin.H{
		"records": records,
		"total":   total,
	}))
}

// ===== Session Recording Handlers =====

func (h *SessionHandler) GetSessionRecordings(c *gin.Context) {
	// 获取当前用户角色
	role, exists := c.Get("role")
	if !exists || role != "admin" {
		c.JSON(http.StatusForbidden, model.Error(403, "权限不足，仅管理员可访问会话审计"))
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	search := c.Query("search")

	sessions, total, err := h.service.GetSessionRecordings(page, pageSize, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.Error(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, model.Success(model.SessionRecordingsResponse{
		Sessions: sessions,
		Total:    total,
	}))
}

func (h *SessionHandler) GetSessionRecording(c *gin.Context) {
	sessionID := c.Param("sessionId")

	recording, err := h.service.GetSessionRecording(sessionID)
	if err != nil {
		logger.Errorf("[Session] GetSessionRecording error: %v", err)
		c.JSON(http.StatusNotFound, model.Error(404, "会话录制不存在"))
		return
	}
	if recording == nil {
		logger.Warnf("[Session] Recording is nil for sessionID: %s", sessionID)
		c.JSON(http.StatusNotFound, model.Error(404, "会话录制不存在"))
		return
	}

	logger.Infof("[Session] GetSessionRecording success, recording.Recording path: %s", recording.Recording)
	logger.Infof("[Session] Recording connectionType: %s", recording.ConnectionType)

	// 对于 RDP 录制，检测文件状态并返回正确的 type
	if recording.ConnectionType == "rdp" {
		playbackURL := fmt.Sprintf("/api/sessions/recordings/%s/file", recording.SessionID)

		// 检测文件状态：优先查找 MP4，其次查找 .guac
		basePath := recording.Recording
		hasMP4 := false
		hasGuac := false
		var mp4Path string
		var guacPath string

		if basePath != "" {
			// 移除可能的扩展名，获取基础路径
			basePathWithoutExt := strings.TrimSuffix(basePath, ".guac")
			basePathWithoutExt = strings.TrimSuffix(basePathWithoutExt, ".mp4")
			basePathWithoutExt = strings.TrimSuffix(basePathWithoutExt, ".cast")

			// 检查 MP4 文件
			mp4Path = basePathWithoutExt + ".mp4"
			if _, err := os.Stat(mp4Path); err == nil {
				hasMP4 = true
				logger.Infof("[Session] Found MP4 file for session %s: %s", recording.SessionID, mp4Path)
			}

			// 检查 .guac 文件
			guacPath = basePathWithoutExt + ".guac"
			if _, err := os.Stat(guacPath); err == nil {
				hasGuac = true
				logger.Infof("[Session] Found .guac file for session %s: %s", recording.SessionID, guacPath)
			}

			// 兼容旧数据：检查无扩展名文件
			if !hasMP4 && !hasGuac {
				if _, err := os.Stat(basePathWithoutExt); err == nil {
					hasGuac = true
					guacPath = basePathWithoutExt
					logger.Infof("[Session] Found recording file without extension for session %s: %s", recording.SessionID, guacPath)
				}
			}
		}

		// 根据文件状态返回不同的 type 和 status
		var recordingType string
		var status string
		var src string

		if hasMP4 {
			// 有 MP4 文件，优先使用 MP4 播放
			recordingType = "mp4"
			status = "ready"
			src = playbackURL + "?format=mp4" // 播放 MP4 文件
			logger.Infof("[Session] Session %s: MP4 file available, ready to play", recording.SessionID)
		} else if hasGuac {
			// 只有 .guac 文件，提示转换中（不提供播放 URL，因为前端只显示提示）
			recordingType = "mp4" // 前端期望 MP4 格式
			status = "converting"
			src = "" // 转换中时不提供播放 URL
			logger.Infof("[Session] Session %s: Only .guac file available, MP4 conversion pending", recording.SessionID)
		} else {
			// 没有文件，提示文件不存在
			recordingType = "mp4"
			status = "not_found"
			src = ""
			logger.Warnf("[Session] Session %s: No recording file found (neither .guac nor .mp4)", recording.SessionID)
		}

		// 返回类似 JumpServer 的格式
		responseData := gin.H{
			"type":      recordingType, // 前端期望 MP4 格式
			"status":    status,        // ready, converting, not_found
			"src":       src,
			"sessionId": recording.SessionID,
			"userId":    recording.UserID,
			"hostId":    recording.HostID,
			"hostName":  recording.HostName,
			"hostIp":    recording.HostIP,
			"username":  recording.Username,
			"startTime": recording.StartTime,
			"endTime":   recording.EndTime,
			"duration":  recording.Duration,
		}
		c.JSON(http.StatusOK, model.Success(responseData))
		return
	}

	// 对于 SSH 录制，返回原始数据（asciinema 格式）
	c.JSON(http.StatusOK, model.Success(recording))
}

// GetSessionRecordingFile 下载/返回会话录制文件（用于 RDP guacd 录制）
func (h *SessionHandler) GetSessionRecordingFile(c *gin.Context) {
	sessionID := c.Param("sessionId")
	logger.Infof("[Session] GetSessionRecordingFile called, sessionID: %s", sessionID)

	recording, err := h.service.GetSessionRecording(sessionID)
	if err != nil {
		logger.Errorf("[Session] GetSessionRecording error: %v", err)
		c.JSON(http.StatusNotFound, model.Error(404, "会话录制不存在或未生成"))
		return
	}
	if recording == nil {
		logger.Warnf("[Session] Recording is nil for sessionID: %s", sessionID)
		c.JSON(http.StatusNotFound, model.Error(404, "会话录制不存在或未生成"))
		return
	}
	if recording.Recording == "" {
		logger.Warnf("[Session] Recording path is empty for sessionID: %s", sessionID)
		c.JSON(http.StatusNotFound, model.Error(404, "会话录制不存在或未生成"))
		return
	}

	// 录制字段存的是容器内路径（/replay/2026/01/05/sessionID_username.guac）
	// 数据库存储的路径与传给 guacd 的路径保持一致
	// 文件名格式：sessionID_username.guac（与录制时传给 guacd 的 recording-name 保持一致）
	basePath := recording.Recording
	logger.Infof("[Session] Recording base path from DB: %s (expected format: /replay/YYYY/MM/DD/sessionID_username.guac)", basePath)

	// 检查请求的格式参数
	format := c.DefaultQuery("format", "mp4") // 默认请求 MP4
	logger.Infof("[Session] Requested format: %s", format)

	// 如果路径不是以 /replay 开头，可能是旧数据，需要转换
	// 但正常情况下，数据库应该存储容器内路径
	var containerPath string
	if strings.HasPrefix(basePath, "/replay") {
		containerPath = basePath
		logger.Infof("[Session] Path is container path, using as-is")
	} else {
		// 兼容旧数据：如果是宿主机路径，转换为容器内路径
		logger.Warnf("[Session] Recording path is not container path, converting: %s", basePath)
		containerBasePath := os.Getenv("RECORDING_CONTAINER_PATH")
		if containerBasePath == "" {
			containerBasePath = "/replay"
		}
		// 提取日期和文件名部分
		parts := strings.Split(strings.ReplaceAll(basePath, "\\", "/"), "/")
		var dateParts []string
		var fileName string

		if len(parts) > 0 {
			fileName = parts[len(parts)-1]
		}

		// 查找日期部分（年/月/日）
		if len(parts) >= 4 {
			for i := len(parts) - 4; i >= 0 && i < len(parts)-1; i++ {
				if i+3 < len(parts) {
					year := parts[i]
					month := parts[i+1]
					day := parts[i+2]
					if len(year) == 4 && len(month) == 2 && len(day) == 2 {
						dateParts = []string{year, month, day}
						break
					}
				}
			}
		}

		if len(dateParts) == 3 {
			containerPath = filepath.Join(containerBasePath, dateParts[0], dateParts[1], dateParts[2], fileName)
		} else {
			containerPath = filepath.Join(containerBasePath, fileName)
		}
		logger.Infof("[Session] Converted container path: %s", containerPath)
	}

	// 根据请求的格式查找文件
	// 移除可能的扩展名，获取基础路径
	basePathWithoutExt := strings.TrimSuffix(containerPath, ".guac")
	basePathWithoutExt = strings.TrimSuffix(basePathWithoutExt, ".mp4")
	basePathWithoutExt = strings.TrimSuffix(basePathWithoutExt, ".cast")

	var path string

	if format == "mp4" {
		// 优先查找 MP4 文件
		mp4Path := basePathWithoutExt + ".mp4"
		if _, err := os.Stat(mp4Path); err == nil {
			path = mp4Path
			logger.Infof("[Session] Found MP4 file at: %s", path)
		} else {
			// MP4 不存在，返回 404（前端会显示转换中提示）
			logger.Warnf("[Session] MP4 file not found: %s", mp4Path)
			c.JSON(http.StatusNotFound, model.Error(404, "MP4 文件不存在，可能正在转换中"))
			return
		}
	} else if format == "guac" {
		// 查找 .guac 文件
		guacPath := basePathWithoutExt + ".guac"
		if _, err := os.Stat(guacPath); err == nil {
			path = guacPath
			logger.Infof("[Session] Found .guac file at: %s", path)
		} else {
			// 兼容旧数据：检查无扩展名文件
			if _, err := os.Stat(basePathWithoutExt); err == nil {
				path = basePathWithoutExt
				logger.Infof("[Session] Found recording file without extension at: %s", path)
			} else {
				logger.Warnf("[Session] .guac file not found: %s", guacPath)
				c.JSON(http.StatusNotFound, model.Error(404, fmt.Sprintf("录制文件不存在: %s", basePath)))
				return
			}
		}
	} else {
		// 未知格式
		logger.Warnf("[Session] Unknown format requested: %s", format)
		c.JSON(http.StatusBadRequest, model.Error(400, fmt.Sprintf("不支持的格式: %s", format)))
		return
	}

	// 尝试打开文件
	f, err := os.Open(path)
	if err != nil {
		logger.Errorf("[Session] Failed to open recording file: %v", err)
		c.JSON(http.StatusNotFound, model.Error(404, fmt.Sprintf("无法打开录制文件: %v", err)))
		return
	}
	defer f.Close()

	// 获取文件信息
	fileInfo, err := f.Stat()
	if err != nil {
		logger.Warnf("[Session] Failed to get file info: %v", err)
	} else {
		logger.Infof("[Session] Recording file size: %d bytes", fileInfo.Size())
	}

	// guacamole_cast 格式就是纯文本的 Guacamole 协议指令流
	// 格式：opcode.length1.value1,length2.value2,...;
	// 检测文件格式：先读取文件开头判断是否为 MP4
	buffer := make([]byte, 12)
	n, err := f.Read(buffer)
	if err != nil && err != io.EOF {
		logger.Errorf("[Session] Failed to read file header: %v", err)
	} else if n >= 8 {
		// MP4 文件开头是 ftyp box，签名是 "ftyp" (从第 4 个字节开始)
		// 或者检查文件扩展名
		isMP4 := strings.HasSuffix(strings.ToLower(path), ".mp4") ||
			(n >= 8 && string(buffer[4:8]) == "ftyp") // MP4 文件签名

		if isMP4 {
			logger.Infof("[Session] Recording file is MP4 format, serving as video file")
			c.Header("Content-Type", "video/mp4")
		} else {
			// 检测是否为 guacamole_cast 格式（纯文本协议指令流）
			// guacamole_cast 格式通常以数字开头，例如 "4.size,2.-6,2.32"
			// 重置文件指针到开头
			f.Seek(0, 0)
			headerBuffer := make([]byte, 50)
			headerN, _ := f.Read(headerBuffer)
			headerStr := string(headerBuffer[:headerN])
			// guacamole_cast 格式特征：包含数字、点、逗号、分号
			isGuacamoleCast := strings.Contains(headerStr, ".") &&
				(strings.Contains(headerStr, ",") || strings.Contains(headerStr, ";"))

			if isGuacamoleCast {
				logger.Infof("[Session] Recording file is guacamole_cast format, serving as text file")
				c.Header("Content-Type", "text/plain; charset=utf-8")
			} else {
				logger.Warnf("[Session] Unknown recording format, defaulting to text/plain")
				c.Header("Content-Type", "text/plain; charset=utf-8")
			}
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
			c.Header("Accept-Ranges", "bytes")

			if _, err := f.Seek(0, 0); err != nil {
				logger.Errorf("[Session] Failed to seek to file start: %v", err)
				c.JSON(http.StatusInternalServerError, model.Error(500, fmt.Sprintf("无法重置文件指针: %v", err)))
				return
			}

			c.Writer.WriteHeader(http.StatusOK)
			_, err = io.Copy(c.Writer, f)
			if err != nil {
				logger.Errorf("Error streaming MP4 recording file: %v", err)
			}
			return
		}
	}

	// 如果不是 MP4，重置文件指针继续处理其他格式
	if _, err := f.Seek(0, 0); err != nil {
		logger.Errorf("[Session] Failed to seek to file start: %v", err)
	}

	// guacamole_cast 格式：直接传输文本内容
	// 先读取文件开头内容用于调试
	previewBuffer := make([]byte, 200)
	previewN, err := f.Read(previewBuffer)
	if err != nil && err != io.EOF {
		logger.Errorf("[Session] Failed to read file for preview: %v", err)
	} else if previewN > 0 {
		// 输出文件开头内容用于调试
		preview := string(previewBuffer[:previewN])
		logger.Infof("[Session] File preview (first %d bytes): %q", previewN, preview)

		// 检查是否有 BOM 或无效字符
		if previewN >= 3 && previewBuffer[0] == 0xEF && previewBuffer[1] == 0xBB && previewBuffer[2] == 0xBF {
			logger.Warnf("[Session] WARNING: File contains UTF-8 BOM, this may cause parsing errors!")
		}

		// 检查第一个有效字符
		startIdx := 0
		if previewN >= 3 && previewBuffer[0] == 0xEF && previewBuffer[1] == 0xBB && previewBuffer[2] == 0xBF {
			startIdx = 3
		}
		for startIdx < previewN && (previewBuffer[startIdx] == ' ' || previewBuffer[startIdx] == '\n' || previewBuffer[startIdx] == '\r' || previewBuffer[startIdx] == '\t') {
			startIdx++
		}
		if startIdx < previewN {
			firstChar := previewBuffer[startIdx]
			logger.Infof("[Session] First valid character: %c (code: %d) at position %d", firstChar, firstChar, startIdx)
		}
	}

	// 重置文件指针到开头
	if _, err := f.Seek(0, 0); err != nil {
		logger.Errorf("[Session] Failed to seek to file start: %v", err)
		c.JSON(http.StatusInternalServerError, model.Error(500, fmt.Sprintf("无法重置文件指针: %v", err)))
		return
	}

	// 跳过 BOM 和开头的空白字符，确保从有效的协议指令开始
	skipBytes := 0

	// 跳过 UTF-8 BOM
	if n >= 3 && buffer[0] == 0xEF && buffer[1] == 0xBB && buffer[2] == 0xBF {
		skipBytes = 3
		logger.Infof("[Session] Detected UTF-8 BOM, will skip it")
	}

	// 跳过开头的空白字符（空格、换行、制表符等）
	// 这些字符会导致 Guacamole 解析器出错
	for skipBytes < n && (buffer[skipBytes] == ' ' || buffer[skipBytes] == '\n' || buffer[skipBytes] == '\r' || buffer[skipBytes] == '\t') {
		skipBytes++
	}

	if skipBytes > 0 {
		logger.Infof("[Session] Skipping %d leading bytes (BOM/whitespace), starting from byte %d", skipBytes, skipBytes)
		if _, err := f.Seek(int64(skipBytes), 0); err != nil {
			logger.Errorf("[Session] Failed to skip leading bytes: %v", err)
			c.JSON(http.StatusInternalServerError, model.Error(500, fmt.Sprintf("无法跳过文件开头的字节: %v", err)))
			return
		}
	}

	// 设置正确的 Content-Type 用于 Guacamole 播放
	// guacamole_cast 格式是纯文本的 Guacamole 协议指令流
	// StaticHTTPTunnel 期望纯文本响应，不能有任何额外的字符或格式
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	// 确保没有额外的响应头干扰
	c.Header("X-Content-Type-Options", "nosniff")

	// 重要：先设置状态码，再传输内容
	// 确保文件从正确的位置开始传输（跳过 BOM/空白字符后）
	c.Writer.WriteHeader(http.StatusOK)

	// 直接流式传输文件内容给 StaticHTTPTunnel
	// StaticHTTPTunnel 会逐字节读取并解析 Guacamole 协议指令
	_, err = io.Copy(c.Writer, f)
	if err != nil {
		// 如果写入失败，记录错误但不返回 JSON（响应头已发送）
		logger.Errorf("Error streaming recording file: %v", err)
	}
}

func (h *SessionHandler) CreateSessionRecording(c *gin.Context) {
	var recording model.SessionRecording
	if err := c.ShouldBindJSON(&recording); err != nil {
		c.JSON(http.StatusBadRequest, model.Error(400, err.Error()))
		return
	}

	if err := h.service.CreateSessionRecording(&recording); err != nil {
		c.JSON(http.StatusInternalServerError, model.Error(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, model.Success(recording))
}

// ===== Command Record Handlers =====
// 使用 command_histories 表（从 linux-proxy 同步的数据）

func (h *SessionHandler) GetCommandRecords(c *gin.Context) {
	// 获取当前用户角色
	role, exists := c.Get("role")
	if !exists || role != "admin" {
		c.JSON(http.StatusForbidden, model.Error(403, "权限不足，仅管理员可访问命令审计"))
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	search := c.Query("search")
	hostFilter := c.DefaultQuery("host", "all")

	commands, total, err := h.service.GetCommandRecords(page, pageSize, search, hostFilter)
	if err != nil {
		log.Printf("[SessionHandler] ERROR: Failed to get command records: %v", err)
		c.JSON(http.StatusInternalServerError, model.Error(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, model.Success(model.CommandRecordsResponse{
		Commands: commands,
		Total:    total,
	}))
}

func (h *SessionHandler) CreateCommandRecord(c *gin.Context) {
	var record model.CommandRecord
	if err := c.ShouldBindJSON(&record); err != nil {
		c.JSON(http.StatusBadRequest, model.Error(400, err.Error()))
		return
	}

	if err := h.service.CreateCommandRecord(&record); err != nil {
		c.JSON(http.StatusInternalServerError, model.Error(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, model.Success(record))
}

func (h *SessionHandler) GetCommandsBySession(c *gin.Context) {
	sessionID := c.Param("sessionId")

	commands, err := h.service.GetCommandsBySession(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.Error(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, model.Success(commands))
}

// TerminateSession 终止会话
func (h *SessionHandler) TerminateSession(c *gin.Context) {
	sessionID := c.Param("sessionId")

	if err := h.service.TerminateSession(sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, model.Error(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, model.Success(gin.H{
		"message": "会话已成功终止",
	}))
}
