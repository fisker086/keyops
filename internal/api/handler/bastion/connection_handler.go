package bastion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fisker/zjump-backend/internal/bastion/blacklist"
	"github.com/fisker/zjump-backend/internal/bastion/parser"
	"github.com/fisker/zjump-backend/internal/bastion/protocol"
	"github.com/fisker/zjump-backend/internal/bastion/recorder"
	"github.com/fisker/zjump-backend/internal/bastion/storage"
	"github.com/fisker/zjump-backend/internal/model"
	"github.com/fisker/zjump-backend/internal/notification"
	"github.com/fisker/zjump-backend/internal/repository"
	"github.com/fisker/zjump-backend/internal/routing"
	authService "github.com/fisker/zjump-backend/internal/service/auth"
	bastionService "github.com/fisker/zjump-backend/internal/service/bastion"
	"github.com/fisker/zjump-backend/pkg/database"
	"github.com/fisker/zjump-backend/pkg/sshclient"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024 * 32,
	WriteBufferSize: 1024 * 32,
	CheckOrigin: func(r *http.Request) bool {
		return true // 在生产环境应该验证 Origin
	},
}

// ConnectionHandler 连接处理器 - 统一入口（支持直连和代理）
type ConnectionHandler struct {
	router         *routing.ConnectionRouter
	hostRepo       *repository.HostRepository
	authSvc        *authService.AuthService
	storage        storage.Storage
	blacklistMgr   *blacklist.Manager
	systemUserRepo *repository.SystemUserRepository
	settingRepo    *repository.SettingRepository
}

// NewConnectionHandler 创建连接处理器
func NewConnectionHandler(
	r *routing.ConnectionRouter,
	hostRepo *repository.HostRepository,
	authSvc *authService.AuthService,
	st storage.Storage,
	db *gorm.DB,
	notificationMgr *notification.NotificationManager,
	systemUserRepo *repository.SystemUserRepository,
	settingRepo *repository.SettingRepository,
) *ConnectionHandler {
	// 初始化黑名单管理器（从数据库读取，带高级检测防绕过）
	blacklistMgr := blacklist.NewManagerFromDB(db)
	blacklistMgr.Start() // 启动定期刷新

	// 连接通知管理器到黑名单管理器（使用传入的共享实例）
	if notificationMgr != nil {
		blacklistMgr.SetNotificationManager(notificationMgr)
	}

	return &ConnectionHandler{
		router:         r,
		hostRepo:       hostRepo,
		authSvc:        authSvc,
		storage:        st,
		blacklistMgr:   blacklistMgr,
		systemUserRepo: systemUserRepo,
		settingRepo:    settingRepo,
	}
}

// HandleConnection 处理 WebSocket 连接（统一入口）
func (h *ConnectionHandler) HandleConnection(c *gin.Context) {
	// 1. 获取参数
	hostID := c.Query("hostId")
	token := c.Query("token")
	systemUserID := c.Query("systemUserId") // 系统用户ID（可选）
	// 获取分辨率参数（用于RDP）
	width := c.Query("width")
	height := c.Query("height")

	log.Printf("[Connection] WebSocket connection request: hostID=%s, systemUserID=%s", hostID, systemUserID)

	if hostID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing hostId parameter"})
		return
	}

	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing token parameter"})
		return
	}

	// 2. 验证 Token 获取用户信息
	userInfo, err := h.validateToken(token)
	if err != nil {
		log.Printf("[Connection] Token validation failed: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
		return
	}

	log.Printf("[Connection] User %s requesting connection to host %s", userInfo.Username, hostID)

	// 1. 先获取主机信息（用于确定协议类型和过滤系统用户）
	host, err := h.hostRepo.FindByID(hostID)
	if err != nil {
		log.Printf("[Connection] Host not found: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Host not found"})
		return
	}

	// 2.1 检查并获取系统用户
	var systemUser *model.SystemUser
	if systemUserID != "" {
		// 如果指定了系统用户ID，验证权限并获取
		hasPermission, err := h.systemUserRepo.CheckUserHasPermission(userInfo.UserID, hostID, systemUserID)
		if err != nil {
			log.Printf("[Connection] Failed to check permission: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check permission"})
			return
		}
		if !hasPermission {
			log.Printf("[Connection] User %s has no permission to use system user %s", userInfo.Username, systemUserID)
			c.JSON(http.StatusForbidden, gin.H{"error": "No permission to use this system user"})
			return
		}

		systemUser, err = h.systemUserRepo.FindByID(systemUserID)
		if err != nil {
			log.Printf("[Connection] System user not found: %v", err)
			c.JSON(http.StatusNotFound, gin.H{"error": "System user not found"})
			return
		}

		// 验证系统用户的协议类型是否匹配设备类型
		requiredProtocol := "ssh"
		if host.DeviceType == model.DeviceTypeWindows {
			requiredProtocol = "rdp"
		}
		if systemUser.Protocol != requiredProtocol {
			log.Printf("[Connection] System user protocol mismatch: host requires %s, system user is %s", requiredProtocol, systemUser.Protocol)
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("System user protocol mismatch. Host type requires %s protocol.", requiredProtocol)})
			return
		}
	} else {
		// 2.2 没有指定系统用户，获取所有可用的系统用户（过滤协议类型）
		requiredProtocol := "ssh"
		if host.DeviceType == model.DeviceTypeWindows {
			requiredProtocol = "rdp"
		}

		// 获取用户所有有权限的系统用户（过滤协议类型）
		systemUsers, err := h.systemUserRepo.GetAvailableSystemUsersForUser(userInfo.UserID, hostID)
		if err != nil {
			log.Printf("[Connection] Failed to get system users: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get system users"})
			return
		}

		// 过滤出匹配协议类型的系统用户
		filteredSystemUsers := make([]model.SystemUser, 0)
		for _, su := range systemUsers {
			if su.Protocol == requiredProtocol {
				filteredSystemUsers = append(filteredSystemUsers, su)
			}
		}

		if len(filteredSystemUsers) == 0 {
			log.Printf("[Connection] No available system users for user %s on host %s (protocol=%s)",
				userInfo.Username, hostID, requiredProtocol)
			c.JSON(http.StatusForbidden, gin.H{
				"error": fmt.Sprintf("No available system users for this host (protocol=%s)", requiredProtocol),
			})
			return
		} else if len(filteredSystemUsers) == 1 {
			// 只有一个系统用户，直接使用
			systemUser = &filteredSystemUsers[0]
			log.Printf("[Connection] Auto-selected system user: %s (username=%s, protocol=%s)",
				systemUser.Name, systemUser.Username, systemUser.Protocol)
		} else {
			// 有多个系统用户，需要前端选择
			log.Printf("[Connection] Multiple system users available (%d) for protocol %s, need user selection",
				len(filteredSystemUsers), requiredProtocol)
			c.JSON(http.StatusOK, gin.H{
				"needSelection": true,
				"systemUsers":   filteredSystemUsers,
			})
			return
		}
	}

	log.Printf("[Connection] Using system user: %s (%s)", systemUser.Name, systemUser.Username)

	// 3. 路由决策
	decision, err := h.router.MakeRoutingDecision(hostID, userInfo.UserID, userInfo.Username)
	if err != nil {
		log.Printf("[Connection] Routing decision failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Routing failed: %v", err)})
		return
	}

	log.Printf("[Connection] Routing decision: mode=%s, reason=%s", decision.Mode, decision.Reason)

	// 4. 升级到 WebSocket
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[Connection] Failed to upgrade to WebSocket: %v", err)
		return
	}
	defer ws.Close()

	// 生成会话ID
	sessionID := uuid.New().String()

	// 5. 根据决策模式建立连接
	if decision.Mode == model.ConnectionModeDirect {
		log.Printf("[Connection] Using DIRECT mode for session %s", sessionID)
		h.handleDirectConnection(ws, hostID, sessionID, userInfo, systemUser, width, height)
	} else {
		log.Printf("[Connection] Using PROXY mode for session %s (proxy: %s)", sessionID, decision.ProxyID)
		h.handleProxyConnection(ws, hostID, sessionID, userInfo, decision, systemUser)
	}
}

// handleDirectConnection 直接连接主机
func (h *ConnectionHandler) handleDirectConnection(ws *websocket.Conn, hostID string, sessionID string, userInfo *UserInfo, systemUser *model.SystemUser, width string, height string) {
	// 获取主机信息
	host, err := h.hostRepo.FindByID(hostID)
	if err != nil {
		log.Printf("[Connection] Host not found: %v", err)
		ws.WriteJSON(map[string]interface{}{
			"type":    "error",
			"message": "Host not found",
		})
		return
	}

	log.Printf("[Connection] Connecting directly to %s (%s:%d) - login user: %s, system user: %s (%s)",
		host.Name, host.IP, host.Port, userInfo.Username, systemUser.Name, systemUser.Username)

	// 发送连接开始消息
	ws.WriteJSON(map[string]interface{}{
		"type":    "info",
		"message": fmt.Sprintf("正在直连到 %s (%s:%d)...", host.Name, host.IP, host.Port),
	})

	// 加载 Windows/RDP 全局配置（guacd、录制等）
	windowsCfg := h.loadWindowsSettings()

	// 创建会话录制器
	rec := recorder.NewRecorder(sessionID, 120, 30)
	connectionSuccess := false // 标记连接是否成功
	startTime := time.Now()
	protocolType := protocol.ProtocolSSH

	// 先创建登录记录（无论连接成功与否都需要记录）- 使用model包
	loginRecord := &model.LoginRecord{
		ID:        sessionID,
		SessionID: sessionID,
		UserID:    userInfo.UserID,
		HostID:    host.ID,
		HostName:  host.Name, // 添加主机名
		HostIP:    host.IP,
		Username:  userInfo.Username,
		LoginTime: startTime,
		Status:    "connecting", // 初始状态为连接中
	}
	if err := database.DB.Create(loginRecord).Error; err != nil {
		log.Printf("[Connection] Failed to create login record: %v", err)
	}
	log.Printf("[Connection] Login record created: session=%s, host=%s(%s), user=%s",
		sessionID, host.Name, host.IP, userInfo.Username)

	// 预先创建会话录制记录，确保后续更新能成功
	if host.DeviceType == model.DeviceTypeWindows {
		protocolType = protocol.ProtocolRDP
	}
	connectionType := map[bool]string{true: "rdp", false: "ssh_client"}[protocolType == protocol.ProtocolRDP]

	// 注意：session_id 是唯一键，如果已存在则更新，不存在则创建
	// 使用 FirstOrCreate 避免记录不存在时的错误日志
	var existingRecording model.SessionRecording
	result := database.DB.Where("session_id = ?", sessionID).FirstOrCreate(&existingRecording, model.SessionRecording{
		ID:             uuid.New().String(),
		SessionID:      sessionID,
		ConnectionType: connectionType,
		UserID:         userInfo.UserID,
		HostID:         host.ID,
		HostName:       host.Name,
		HostIP:         host.IP,
		Username:       userInfo.Username,
		StartTime:      startTime,
		Status:         "connecting",
		Duration:       "进行中",
	})

	if result.Error != nil {
		log.Printf("[Connection] Failed to create or query session recording: %v", result.Error)
	} else {
		// 如果记录已存在，更新连接类型和状态
		if result.RowsAffected == 0 {
			// 记录已存在，更新连接类型和状态
			updates := map[string]interface{}{
				"connection_type": connectionType,
				"status":          "connecting",
				"start_time":      startTime,
			}
			if err := database.DB.Model(&existingRecording).Updates(updates).Error; err != nil {
				log.Printf("[Connection] Failed to update session recording: %v", err)
			}
		}
	}

	// 记录/回放数据占位
	recordingData := ""

	defer func() {
		rec.Close()
		logoutTime := time.Now()
		diff := logoutTime.Sub(startTime)
		durationSec := int(diff.Seconds())
		if connectionSuccess && recordingData == "" {
			if ascii, err := rec.ToAsciinema(); err == nil {
				recordingData = ascii
			} else {
				log.Printf("[Connection] Failed to export recording: %v", err)
			}
		}
		recording := recordingData

		log.Printf("[Connection] Session %s ending: success=%v, duration=%ds, host=%s",
			sessionID, connectionSuccess, durationSec, host.Name)

		if connectionSuccess {
			// 连接成功，更新会话录制记录和登录记录
			// 1. 更新会话录制记录（添加录像数据）
			minutes := int(diff.Minutes())
			seconds := int(diff.Seconds()) % 60
			duration := fmt.Sprintf("%dm %ds", minutes, seconds)

			result := database.DB.Model(&model.SessionRecording{}).
				Where("session_id = ?", sessionID).
				Updates(map[string]interface{}{
					"end_time":  logoutTime,
					"status":    "closed",
					"duration":  duration,
					"recording": recording,
				})
			if result.Error != nil {
				log.Printf("[Connection]  Failed to update session recording: %v", result.Error)
			} else {
				log.Printf("[Connection]  Session recording updated: session=%s, affected_rows=%d",
					sessionID, result.RowsAffected)
			}

			// 2. 更新登录记录为完成状态
			result = database.DB.Model(&model.LoginRecord{}).
				Where("session_id = ?", sessionID).
				Updates(map[string]interface{}{
					"logout_time": logoutTime,
					"status":      "completed",
					"duration":    durationSec,
				})
			if result.Error != nil {
				log.Printf("[Connection]  Failed to update login record to completed: %v", result.Error)
			} else {
				log.Printf("[Connection]  Login record updated to completed: session=%s, affected_rows=%d",
					sessionID, result.RowsAffected)
			}

			log.Printf("[Connection]  Session %s closed successfully (duration: %v)", sessionID, duration)
		} else {
			// 连接失败，只更新登录记录
			result := database.DB.Model(&model.LoginRecord{}).
				Where("session_id = ?", sessionID).
				Updates(map[string]interface{}{
					"logout_time": logoutTime,
					"status":      "failed",
					"duration":    durationSec,
				})
			if result.Error != nil {
				log.Printf("[Connection]  Failed to update login record to failed: %v", result.Error)
			} else {
				log.Printf("[Connection]  Login record updated to failed: session=%s, duration=%ds, affected_rows=%d",
					sessionID, durationSec, result.RowsAffected)
			}

			log.Printf("[Connection]  Session %s failed (duration: %ds)", sessionID, durationSec)
		}
	}()

	// 根据设备类型自动选择协议
	// Windows 设备使用 RDP，其他设备（Linux、交换机等）使用 SSH
	if protocolType == protocol.ProtocolRDP {
		log.Printf("[Connection] Device type is Windows, using RDP protocol")
	} else {
		log.Printf("[Connection] Device type is %s, using SSH protocol", host.DeviceType)
	}

	// 如果是 RDP，使用协议处理器
	// 注意：RDP 使用 Guacamole 协议，不能发送 JSON 消息，否则会导致解析错误
	if protocolType == protocol.ProtocolRDP {
		desiredWidth, desiredHeight := parseResolution(width, height)

		// 录制文件路径（用于审计列表展示下载链接）
		// 注意：数据库存储的是容器内路径（/replay/...），与传给 guacd 的路径保持一致
		// 路径结构：
		// - 都按天组织目录：/replay/2024/01/15/
		// - 文件名都是：sessionID_username
		recPath := ""
		if windowsCfg.RecordingEnabled {
			// normalizeRecordingPath 返回容器内路径（/replay/...）
			// 传入用户名，确保文件名格式与传给 guacd 的一致：sessionID_username
			// 获取容器内路径（从环境变量或使用默认值）
			containerBasePath := os.Getenv("RECORDING_CONTAINER_PATH")
			if containerBasePath == "" {
				containerBasePath = "/replay"
			}
			// 使用界面用户（登录 zjump 的用户）而不是 Windows 登录用户（系统用户）
			recPath = normalizeRecordingPath(containerBasePath, sessionID, userInfo.Username)
			recordingData = recPath
		}

		if err := h.handleRDPConnection(ws, host, systemUser, sessionID, rec, userInfo, &connectionSuccess, startTime, windowsCfg, recPath, desiredWidth, desiredHeight); err != nil {
			log.Printf("[Connection] RDP connection error: %v", err)
			var errorMsg string
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				errorMsg = fmt.Sprintf("\r\n\033[1;31m连接超时！\033[0m\r\n无法连接到 %s:%d\r\n请检查：\r\n1. 主机是否在线\r\n2. 网络是否可达\r\n3. RDP服务是否正常运行\r\n4. 防火墙是否允许连接\r\n", host.IP, host.Port)
			} else {
				errorMsg = fmt.Sprintf("\r\n\033[1;31m连接失败！\033[0m\r\n无法连接到 %s:%d\r\n错误：%v\r\n", host.IP, host.Port, err)
			}
			ws.WriteJSON(map[string]interface{}{
				"type":    "error",
				"message": errorMsg,
			})
			return
		}
	} else {
		// SSH 连接
		// 创建命令解析器，用于记录命令到数据库
		commandParser := parser.NewCommandExtractor(func(cmd string) {
			log.Printf("[Connection] ===== Command detected ===== session=%s, host=%s, user=%s, command=%q", 
				sessionID, host.IP, userInfo.Username, cmd)
			
			// 记录命令到数据库
			commandRecord := &storage.CommandRecord{
				ProxyID:    "api-server-direct",
				SessionID:  sessionID,
				HostID:     host.ID,
				UserID:     userInfo.UserID,
				Username:   userInfo.Username,
				HostIP:     host.IP,
				Command:    cmd,
				ExecutedAt: time.Now(),
			}
			
			log.Printf("[Connection] Preparing to save command record: %+v", commandRecord)
			
			if err := h.storage.SaveCommand(commandRecord); err != nil {
				log.Printf("[Connection] ERROR: Failed to save command: %v", err)
			} else {
				log.Printf("[Connection] SUCCESS: Command saved successfully: %s", cmd)
			}
		})
		
		if err := h.proxySSHConnectionWithTimeout(ws, host, systemUser, sessionID, rec, commandParser, userInfo, &connectionSuccess, startTime); err != nil {
			log.Printf("[Connection] SSH connection error: %v", err)

			// 根据错误类型显示不同的消息
			var errorMsg string
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				errorMsg = fmt.Sprintf("\r\n\033[1;31m连接超时！\033[0m\r\n无法连接到 %s:%d\r\n请检查：\r\n1. 主机是否在线\r\n2. 网络是否可达\r\n3. SSH服务是否正常运行\r\n4. 防火墙是否允许连接\r\n", host.IP, host.Port)
			} else {
				errorMsg = fmt.Sprintf("\r\n\033[1;31m连接失败！\033[0m\r\n错误：%v\r\n", err)
			}

			ws.WriteJSON(map[string]interface{}{
				"type":    "error",
				"message": errorMsg,
			})
			// connectionSuccess 保持 false，defer 中会标记为 failed
		} else {
			// 连接函数正常返回（注意：connectionSuccess 已在Shell启动时设置）
			log.Printf("[Connection] SSH connection function returned normally for session %s", sessionID)
		}
	}
}

// handleProxyConnection 通过代理连接主机
func (h *ConnectionHandler) handleProxyConnection(ws *websocket.Conn, hostID string, sessionID string, userInfo *UserInfo, decision *model.RoutingDecision, systemUser *model.SystemUser) {
	host, err := h.hostRepo.FindByID(hostID)
	if err != nil {
		ws.WriteJSON(map[string]interface{}{
			"type":    "error",
			"message": "Host not found",
		})
		return
	}

	log.Printf("[Connection] Connecting via proxy %s to %s as %s", decision.ProxyID, host.Name, systemUser.Username)

	// 增加登录次数
	if err := h.hostRepo.IncrementLoginCount(host.ID); err != nil {
		log.Printf("[Connection] Failed to increment login count: %v", err)
	}
	if err := h.hostRepo.UpdateLastLoginTime(host.ID); err != nil {
		log.Printf("[Connection] Failed to update last login time: %v", err)
	}

	// 发送连接消息
	ws.WriteJSON(map[string]interface{}{
		"type":    "info",
		"message": fmt.Sprintf("正在通过代理 %s 连接到 %s...", decision.ProxyID, host.Name),
	})

	// 生成 Proxy Token（用于 Proxy Server 验证）
	proxyToken := h.generateProxyToken(hostID, userInfo)

	// 连接到 Proxy Server
	proxyURL := fmt.Sprintf("%s?token=%s&hostId=%s", decision.ProxyURL, proxyToken, hostID)
	log.Printf("[Connection] Dialing proxy: %s", proxyURL)

	proxyWS, _, err := websocket.DefaultDialer.Dial(proxyURL, nil)
	if err != nil {
		log.Printf("[Connection] Failed to connect to proxy: %v", err)
		ws.WriteJSON(map[string]interface{}{
			"type":    "error",
			"message": fmt.Sprintf("无法连接到代理服务器: %v", err),
		})
		return
	}
	defer proxyWS.Close()

	log.Printf("[Connection] Successfully connected to proxy, starting bidirectional forwarding...")

	// 双向转发 WebSocket 数据
	errChan := make(chan error, 2)

	// 客户端 -> 代理
	go func() {
		for {
			messageType, message, err := ws.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := proxyWS.WriteMessage(messageType, message); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// 代理 -> 客户端
	go func() {
		for {
			messageType, message, err := proxyWS.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := ws.WriteMessage(messageType, message); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// 等待任一方向发生错误
	err = <-errChan
	if err != nil && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		log.Printf("[Connection] Proxy forwarding error: %v", err)
	}

	log.Printf("[Connection] Session %s closed (proxy mode)", sessionID)
}

// proxySSHConnectionWithTimeout 代理 SSH 连接（带超时倒计时）
func (h *ConnectionHandler) proxySSHConnectionWithTimeout(ws *websocket.Conn, host *model.Host, systemUser *model.SystemUser, sessionID string, rec *recorder.Recorder, cmdParser *parser.CommandExtractor, userInfo *UserInfo, connectionSuccess *bool, startTime time.Time) error {
	// 创建超时上下文（改为30秒）
	timeout := 30 * time.Second
	deadline := time.Now().Add(timeout)

	// 创建用于取消倒计时的通道
	stopCountdown := make(chan struct{})

	// 用于记录是否显示过倒计时
	countdownShown := false

	// 启动倒计时显示
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopCountdown:
				// 连接成功，只在显示过倒计时时才清除
				if countdownShown {
					ws.WriteJSON(map[string]interface{}{
						"type": "data",
						"data": "\r\033[K", // 清除当前行
					})
				}
				log.Printf("[Connection] Countdown stopped for session %s", sessionID)
				return
			case <-ticker.C:
				remaining := time.Until(deadline)
				if remaining > 0 {
					countdownShown = true
					ws.WriteJSON(map[string]interface{}{
						"type": "data",
						"data": fmt.Sprintf("\r\033[33m正在连接... 剩余时间: %d 秒\033[0m", int(remaining.Seconds())),
					})
				}
			}
		}
	}()

	// 执行实际的SSH连接，传递stopCountdown通道、connectionSuccess指针和startTime
	log.Printf("[Connection] Starting SSH connection with timeout: %v", timeout)
	return h.proxySSHConnection(ws, host, systemUser, sessionID, rec, cmdParser, userInfo, stopCountdown, connectionSuccess, startTime)
}

// proxySSHConnection 代理 SSH 连接（直连模式）
func (h *ConnectionHandler) proxySSHConnection(ws *websocket.Conn, host *model.Host, systemUser *model.SystemUser, sessionID string, rec *recorder.Recorder, cmdParser *parser.CommandExtractor, userInfo *UserInfo, stopCountdown chan struct{}, connectionSuccess *bool, startTime time.Time) error {
	// 使用系统用户的认证信息
	// 注意：Host 已不再包含认证字段，必须通过 SystemUser 提供
	username := systemUser.Username
	password := systemUser.Password
	privateKey := systemUser.PrivateKey
	passphrase := systemUser.Passphrase
	authType := systemUser.AuthType

	// 验证系统用户必须配置了对应认证类型的认证信息
	if authType == "password" && password == "" {
		return fmt.Errorf("系统用户 %s 配置为密码认证，但未提供密码", systemUser.Name)
	}
	if authType == "key" && privateKey == "" {
		return fmt.Errorf("系统用户 %s 配置为密钥认证，但未提供私钥", systemUser.Name)
	}
	if authType == "both" && password == "" && privateKey == "" {
		return fmt.Errorf("系统用户 %s 配置为同时支持密码和密钥认证，但未提供密码或私钥", systemUser.Name)
	}

	cfg := sshclient.SSHConfig{
		Host:       host.IP,
		Port:       host.Port,
		Username:   username,
		Password:   password,
		PrivateKey: privateKey,
		Passphrase: passphrase,
		AuthType:   authType,
		Timeout:    30 * time.Second,
	}

	log.Printf("[Connection] SSH Config: Host=%s, Port=%d, Username=%s, AuthType=%s, HasPassword=%v, HasPrivateKey=%v, Timeout=%v",
		cfg.Host, cfg.Port, cfg.Username, cfg.AuthType, password != "", privateKey != "", cfg.Timeout)
	log.Printf("[Connection] Attempting SSH connection to %s:%d as %s (system user: %s)",
		host.IP, host.Port, username, systemUser.Name)

	client, err := sshclient.NewSSHClient(cfg)
	if err != nil {
		log.Printf("[Connection] SSH client creation failed: %v", err)
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()
	log.Printf("[Connection] SSH client created successfully for session %s", sessionID)

	// 创建 SSH session
	session, err := client.NewSession()
	if err != nil {
		log.Printf("[Connection] SSH session creation failed: %v", err)
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()
	log.Printf("[Connection] SSH session created successfully for session %s", sessionID)

	// 设置终端模式
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	// 请求 PTY
	if err := session.RequestPty("xterm-256color", 30, 120, modes); err != nil {
		return fmt.Errorf("failed to request pty: %w", err)
	}

	// 获取输入输出管道
	stdin, _ := session.StdinPipe()
	stdout, _ := session.StdoutPipe()
	stderr, _ := session.StderrPipe()

	// 启动 shell
	if err := session.Shell(); err != nil {
		return fmt.Errorf("failed to start shell: %w", err)
	}

	// Shell 启动成功！停止倒计时 + 标记连接成功
	if stopCountdown != nil {
		close(stopCountdown)
		log.Printf("[Connection]  Shell started successfully, countdown stopped for session %s", sessionID)
	}

	// 标记连接成功（非常重要：即使后面WebSocket断开，也要记录这次成功的连接）
	if connectionSuccess != nil {
		*connectionSuccess = true
		log.Printf("[Connection]  Connection marked as successful for session %s", sessionID)

		// 更新会话录制记录状态为 active（记录已在 handleDirectConnection 中创建）
		// 注意：不要在 proxySSHConnection 中创建新记录，因为 handleDirectConnection 已经创建了
		updates := map[string]interface{}{
			"connection_type": "webshell",
			"status":          "active",
			"terminal_cols":   120,
			"terminal_rows":   30,
		}
		result := database.DB.Model(&model.SessionRecording{}).
			Where("session_id = ?", sessionID).
			Updates(updates)

		if result.Error != nil {
			log.Printf("[Connection]  Failed to update session recording: %v", result.Error)
		} else if result.RowsAffected > 0 {
			log.Printf("[Connection]  Session recording updated: session=%s, host=%s, type=webshell, rows_affected=%d",
				sessionID, host.Name, result.RowsAffected)
		} else {
			// 如果记录不存在（理论上不应该发生），尝试创建
			log.Printf("[Connection]  Session recording not found, creating new record for session %s", sessionID)
			recording := &model.SessionRecording{
				ID:             uuid.New().String(),
				SessionID:      sessionID,
				ConnectionType: "webshell",
				UserID:         userInfo.UserID,
				HostID:         host.ID,
				HostName:       host.Name,
				HostIP:         host.IP,
				Username:       userInfo.Username,
				StartTime:      startTime,
				Status:         "active",
				Duration:       "进行中",
				TerminalCols:   120,
				TerminalRows:   30,
			}
			if err := database.DB.Create(recording).Error; err != nil {
				log.Printf("[Connection]  Failed to create session recording: %v", err)
			} else {
				log.Printf("[Connection]  Session recording created: id=%s, session=%s, host=%s, type=webshell",
					recording.ID, sessionID, host.Name)
			}
		}

		// 更新登录记录状态为 active
		if err := database.DB.Model(&model.LoginRecord{}).
			Where("session_id = ?", sessionID).
			Update("status", "active").Error; err != nil {
			log.Printf("[Connection] Failed to update login record status: %v", err)
		}

		// 更新主机统计信息
		if err := h.hostRepo.IncrementLoginCount(host.ID); err != nil {
			log.Printf("[Connection] Failed to increment login count: %v", err)
		}
		if err := h.hostRepo.UpdateLastLoginTime(host.ID); err != nil {
			log.Printf("[Connection] Failed to update last login time: %v", err)
		}
	}

	errChan := make(chan error, 2)

	// WebSocket -> SSH stdin（带命令拦截）
	go func() {
		defer stdin.Close()

		// 命令输入缓冲区（用于在回车前检测完整命令）
		var commandBuffer strings.Builder

		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}

			// 解析消息
			var msg map[string]interface{}
			if err := json.Unmarshal(message, &msg); err != nil {
				stdin.Write(message)
				continue
			}

			msgType, ok := msg["type"].(string)
			if !ok {
				continue
			}

			switch msgType {
			case "resize":
				if cols, ok := msg["cols"].(float64); ok {
					if rows, ok := msg["rows"].(float64); ok {
						session.WindowChange(int(rows), int(cols))
					}
				}
			case "input":
				if data, ok := msg["data"].(string); ok {
					rec.RecordInput(data)

					// 检查是否是回车键（命令执行）
					if data == "\r" || data == "\n" {
						// 获取完整命令
						command := strings.TrimSpace(commandBuffer.String())
						commandLen := len(commandBuffer.String())

						// 检查黑名单（在命令执行前，带通知功能）
						if command != "" && h.blacklistMgr != nil && h.blacklistMgr.IsBlockedWithNotify(command, userInfo.Username, host.IP) {
							reason := h.blacklistMgr.GetBlockReason(command, userInfo.Username)

							// 记录被阻止的命令
							blockedRecord := &storage.CommandRecord{
								ProxyID:    "api-server-direct",
								SessionID:  sessionID,
								HostID:     host.ID,
								UserID:     userInfo.UserID,
								Username:   userInfo.Username,
								HostIP:     host.IP,
								Command:    command,
								Output:     fmt.Sprintf("[BLOCKED] %s", reason),
								ExitCode:   -1,
								ExecutedAt: time.Now(),
							}
							h.storage.SaveCommand(blockedRecord)

							// 清空缓冲区
							commandBuffer.Reset()

							// 发送退格键清除已输入的命令
							for i := 0; i < commandLen; i++ {
								stdin.Write([]byte{0x7f})
							}

							// 发送回车让 shell 显示新提示符（用户不需要再手动按回车）
							stdin.Write([]byte("\r"))

							// 发送阻止警告给客户端
							blockMsg := fmt.Sprintf("\r\n\033[1;31m🛡️ [安全策略阻止] %s\033[0m\r\n", reason)
							ws.WriteJSON(map[string]interface{}{
								"type": "output",
								"data": blockMsg,
							})

							continue
						}

						// 清空缓冲区
						commandBuffer.Reset()

						// 命令安全，正常执行
						stdin.Write([]byte(data))
					} else if data == "\x03" { // Ctrl+C
						// 清空缓冲区
						commandBuffer.Reset()
						stdin.Write([]byte(data))
					} else if data == "\x7f" || data == "\b" { // 退格
						// 从缓冲区删除最后一个字符
						s := commandBuffer.String()
						if len(s) > 0 {
							commandBuffer.Reset()
							commandBuffer.WriteString(s[:len(s)-1])
						}
						stdin.Write([]byte(data))
					} else {
						// 累积到命令缓冲区
						commandBuffer.WriteString(data)
						stdin.Write([]byte(data))
					}
				}
			}
		}
	}()

	// SSH stdout -> WebSocket
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				if err != io.EOF {
					errChan <- err
				}
				return
			}
			if n > 0 {
				data := string(buf[:n])
				rec.RecordOutput(data)
				// 喂给命令解析器解析命令（如果存在）
				if cmdParser != nil {
					log.Printf("[Connection] Feeding data to command parser, length=%d", len(data))
					cmdParser.Feed(data)
				} else {
					log.Printf("[Connection] WARNING: Command parser is nil, command will not be recorded!")
				}
				ws.WriteJSON(map[string]interface{}{
					"type": "output",
					"data": data,
				})
			}
		}
	}()

	// SSH stderr -> WebSocket
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := stderr.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				data := string(buf[:n])
				rec.RecordOutput(data)
				// stderr 也可能包含命令提示符（如果解析器存在）
				if cmdParser != nil {
					log.Printf("[Connection] Feeding stderr data to command parser, length=%d", len(data))
					cmdParser.Feed(data)
				}
				ws.WriteJSON(map[string]interface{}{
					"type": "output",
					"data": data,
				})
			}
		}
	}()

	// 等待连接结束
	return <-errChan
}

// validateToken 验证 Token（使用JWT Token，24小时有效期）
func (h *ConnectionHandler) validateToken(token string) (*UserInfo, error) {
	// 验证 JWT Token（用户登录token，24小时有效期）
	claims, err := h.authSvc.ValidateToken(token)
	if err != nil {
		// 兼容旧的SessionToken方式（可选）
		if tokenInfo, err := bastionService.ValidateSessionToken(token); err == nil {
			return &UserInfo{
				UserID:   tokenInfo.UserID,
				Username: tokenInfo.Username,
			}, nil
		}
		return nil, fmt.Errorf("invalid or expired token: %w", err)
	}

	return &UserInfo{
		UserID:   claims.UserID,
		Username: claims.Username,
	}, nil
}

// generateProxyToken 生成给 Proxy Server 的 Token
func (h *ConnectionHandler) generateProxyToken(hostID string, userInfo *UserInfo) string {
	// TODO: 实现真实的 token 生成
	// 这里简化处理，实际应该生成 JWT
	return "proxy-token-" + hostID + "-" + userInfo.UserID
}

// UserInfo 用户信息
type UserInfo struct {
	UserID   string
	Username string
}

// normalizeRecordingPath 构建录制文件的宿主机路径（用于存储到数据库，供外部程序读取）
// basePath: 基础路径（宿主机路径，通过 volume 挂载映射）
// sessionID: 会话ID
// username: 用户名（RDP 连接的用户名）
// 返回：完整的文件路径（宿主机路径，按天组织目录，文件名格式：sessionID_username）
// 注意：路径结构与传给 guacd 的路径保持一致，只是基础路径不同
func normalizeRecordingPath(basePath string, sessionID string, username string) string {
	// 如果 basePath 为空，使用默认值
	if basePath == "" || basePath == "recordings" {
		basePath = "/tmp/replay"
	}

	// 按天创建目录：/basePath/2024/01/15/
	now := time.Now()
	dayPath := filepath.Join(
		basePath,
		strconv.Itoa(now.Year()),
		fmt.Sprintf("%02d", int(now.Month())),
		fmt.Sprintf("%02d", now.Day()),
	)

	// 文件名：sessionID_username（与传给 guacd 的格式保持一致）
	recordingName := sessionID
	if username != "" {
		recordingName = sessionID + "_" + username
	}

	// 返回完整路径：/basePath/2024/01/15/sessionID_username.guac
	// guacd 使用 guac 格式时会保存为 .guac 扩展名
	return filepath.Join(dayPath, recordingName+".guac")
}

// 从查询参数解析期望的分辨率，返回宽高（<=0 表示未指定）
func parseResolution(widthStr string, heightStr string) (int, int) {
	width := 0
	if w, err := strconv.Atoi(widthStr); err == nil && w > 0 {
		width = w
	}
	height := 0
	if hVal, err := strconv.Atoi(heightStr); err == nil && hVal > 0 {
		height = hVal
	}
	return width, height
}

type windowsSettings struct {
	EnableAccess       bool
	GuacdHost          string
	GuacdPort          int
	RecordingEnabled   bool
	RecordingPath      string
	RecordingFormat    string
	AllowClipboard     bool
	EnableFileTransfer bool
	DrivePath          string
}

func (h *ConnectionHandler) loadWindowsSettings() windowsSettings {
	// defaults：仅用安全兜底，具体由 DB 配置覆盖
	def := windowsSettings{
		EnableAccess:       false,
		GuacdHost:          "localhost",
		GuacdPort:          4822,
		RecordingEnabled:   true,
		RecordingPath:      "/replay",
		RecordingFormat:    "guac",
		AllowClipboard:     false,
		EnableFileTransfer: false,
		DrivePath:          "/replay-drive",
	}

	if h.settingRepo == nil {
		return def
	}

	settings, err := h.settingRepo.GetByCategory("windows")
	if err != nil {
		log.Printf("[Connection] loadWindowsSettings failed, use defaults: %v", err)
		return def
	}

	toBool := func(v string, fallback bool) bool {
		switch strings.ToLower(v) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		default:
			return fallback
		}
	}

	toInt := func(v string, fallback int) int {
		if iv, err := strconv.Atoi(v); err == nil {
			return iv
		}
		return fallback
	}

	result := def
	for _, s := range settings {
		switch s.Key {
		case "enable_windows_access", "enableWindowsAccess":
			result.EnableAccess = toBool(s.Value, result.EnableAccess)
		case "guacd_host", "guacdHost":
			if s.Value != "" {
				result.GuacdHost = s.Value
			}
		case "guacd_port", "guacdPort":
			result.GuacdPort = toInt(s.Value, result.GuacdPort)
		case "recording_enabled", "recordingEnabled":
			result.RecordingEnabled = toBool(s.Value, result.RecordingEnabled)
		case "recording_path", "recordingPath":
			if s.Value != "" {
				result.RecordingPath = s.Value
			}
		case "recording_format", "recordingFormat":
			if s.Value != "" {
				result.RecordingFormat = s.Value
			}
		case "allow_clipboard", "allowClipboard":
			result.AllowClipboard = toBool(s.Value, result.AllowClipboard)
		case "enable_file_transfer", "enableFileTransfer":
			result.EnableFileTransfer = toBool(s.Value, result.EnableFileTransfer)
		case "drive_path", "drivePath":
			if s.Value != "" {
				result.DrivePath = s.Value
			}
		}
	}

	return result
}

// handleRDPConnection 处理 RDP 连接（使用 FreeRDP）
func (h *ConnectionHandler) handleRDPConnection(ws *websocket.Conn, host *model.Host, systemUser *model.SystemUser, sessionID string, rec *recorder.Recorder, userInfo *UserInfo, connectionSuccess *bool, startTime time.Time, winCfg windowsSettings, recordingPath string, desiredWidth int, desiredHeight int) error {
	// 创建录制器适配器
	recorderAdapter := recorder.NewRecorderAdapter(rec)

	// 创建 RDP 协议处理器
	factory := protocol.GetFactory()
	handler, err := factory.Create(protocol.ProtocolRDP, recorderAdapter)
	if err != nil {
		return fmt.Errorf("failed to create RDP handler: %w", err)
	}
	defer handler.Close()

	// 构建连接配置
	rdpPort := host.Port
	if rdpPort == 0 {
		rdpPort = 3389 // RDP 默认端口
	}

	config := &protocol.ConnectionConfig{
		HostID:    host.ID,
		HostIP:    host.IP,
		HostPort:  rdpPort,
		Username:  systemUser.Username,
		Password:  systemUser.Password, // 需要解密（如果加密了）
		Protocol:  protocol.ProtocolRDP,
		SessionID: sessionID,
		UserID:    userInfo.UserID,
		ProxyID:   "api-server-direct",
		Timeout:   30 * time.Second,
		Options:   make(map[string]interface{}),
	}

	// 设置 RDP 选项（优先使用前端传入的分辨率，兜底 1280x800，按 guacd 限制 4096）
	const (
		minWidth      = 1280
		minHeight     = 800
		guacMaxDim    = 4096
		defaultWidth  = 1940
		defaultHeight = 960
	)

	width := desiredWidth
	height := desiredHeight
	if width <= 0 {
		width = defaultWidth
	}
	if height <= 0 {
		height = defaultHeight
	}
	if width < minWidth {
		width = minWidth
	}
	if height < minHeight {
		height = minHeight
	}
	if width > guacMaxDim {
		width = guacMaxDim
	}
	if height > guacMaxDim {
		height = guacMaxDim
	}

	log.Printf("[Connection] RDP resolution request: desired=(%d,%d) -> final=(%d,%d)", desiredWidth, desiredHeight, width, height)

	config.Options["width"] = width
	config.Options["height"] = height
	config.Options["guacd_host"] = winCfg.GuacdHost
	config.Options["guacd_port"] = winCfg.GuacdPort
	config.Options["recording_enabled"] = winCfg.RecordingEnabled
	config.Options["recording_path"] = winCfg.RecordingPath
	config.Options["recording_format"] = winCfg.RecordingFormat
	config.Options["allow_clipboard"] = winCfg.AllowClipboard
	config.Options["enable_file_transfer"] = winCfg.EnableFileTransfer
	config.Options["drive_path"] = winCfg.DrivePath
	// 存储界面用户名（登录 zjump 的用户），用于录制文件名
	config.Options["ui_username"] = userInfo.Username

	// 设置 RDP 安全模式
	// 默认使用 "rdp" 传统 RDP 安全模式，兼容 xrdp 和大多数 RDP 服务器
	// xrdp 服务器（如 satishweb/xrdp Docker 镜像）需要 "rdp" 模式，否则会出现错误 519
	// 如果连接失败，可以尝试以下模式：
	// - "rdp": 传统 RDP 安全（默认，推荐）- 兼容 xrdp、旧 Windows 服务器
	// - "nla": Network Level Authentication（现代 Windows 服务器，更安全）
	// - "tls": TLS 加密
	// - "any": 自动协商（某些服务器可能不支持）
	// TODO: 未来可以通过配置文件或主机配置来覆盖默认值
	config.Options["security"] = "rdp"
	// 建立 RDP 连接
	ctx := context.Background()
	if err := handler.Connect(ctx, config); err != nil {
		return fmt.Errorf("failed to connect to RDP server: %w", err)
	}

	*connectionSuccess = true

	// 处理 WebSocket 连接
	if err := handler.HandleWebSocket(ws); err != nil {
		return fmt.Errorf("RDP WebSocket handling error: %w", err)
	}

	return nil
}
