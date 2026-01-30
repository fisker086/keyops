package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/fisker/zjump-backend/internal/model"
	"github.com/fisker/zjump-backend/internal/repository"
	"github.com/google/uuid"
)

// SessionToken 会话令牌（用于 Proxy 验证）
type SessionToken struct {
	Token     string
	HostID    string
	UserID    string
	Username  string
	ExpiresAt time.Time
}

// 内存存储会话令牌（生产环境可以用 Redis）
var (
	sessionTokens = sync.Map{}
)

type SessionService struct {
	repo     *repository.SessionRepository
	hostRepo *repository.HostRepository
}

func NewSessionService(repo *repository.SessionRepository, hostRepo *repository.HostRepository) *SessionService {
	return &SessionService{
		repo:     repo,
		hostRepo: hostRepo,
	}
}

func (s *SessionService) CreateSession(hostID string, userID string) (*model.SessionResponse, error) {
	host, err := s.hostRepo.FindByID(hostID)
	if err != nil {
		return nil, fmt.Errorf("主机不存在")
	}

	sessionID := uuid.New().String()

	// 创建登录记录（会话记录统一使用 session_recordings 表）
	// TODO: Username 应从系统用户获取，需要扩展 CreateSession 方法签名
	loginRecord := &model.LoginRecord{
		ID:        uuid.New().String(),
		UserID:    userID, // 使用认证上下文中的用户ID
		HostID:    hostID,
		HostName:  host.Name,
		HostIP:    host.IP,
		Username:  "", // TODO: 从系统用户获取
		LoginTime: time.Now(),
		Status:    "active",
		SessionID: sessionID,
	}

	if err := s.repo.CreateLoginRecord(loginRecord); err != nil {
		return nil, err
	}

	// 生成临时令牌（用于 Proxy 验证）
	token := generateSessionToken()

	// 存储令牌信息（5分钟有效期）
	// TODO: Username 应从系统用户获取
	tokenInfo := &SessionToken{
		Token:     token,
		HostID:    hostID,
		UserID:    userID, // 使用真实用户ID
		Username:  "",     // TODO: 从系统用户获取
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	sessionTokens.Store(token, tokenInfo)

	// 清理过期令牌（异步）
	go cleanExpiredTokens()

	// 根据设备类型选择 Proxy 端口和协议
	proxyPort := 8022 // 默认 Linux SSH Proxy
	protocol := "ssh"

	// TODO: 根据 host.DeviceType 和 host.Protocol 选择对应的 Proxy
	// 可以从配置文件或数据库读取 Proxy 地址

	wsURL := fmt.Sprintf("ws://localhost:%d/ws/%s?token=%s", proxyPort, protocol, token)

	return &model.SessionResponse{
		SessionID: sessionID,
		WSUrl:     wsURL,
		Token:     token, // 返回令牌给前端
	}, nil
}

// ValidateSessionToken 验证会话令牌（供 Proxy 调用）
func ValidateSessionToken(token string) (*SessionToken, error) {
	value, ok := sessionTokens.Load(token)
	if !ok {
		return nil, fmt.Errorf("invalid token")
	}

	tokenInfo := value.(*SessionToken)

	// 检查是否过期
	if time.Now().After(tokenInfo.ExpiresAt) {
		sessionTokens.Delete(token)
		return nil, fmt.Errorf("token expired")
	}

	return tokenInfo, nil
}

// generateSessionToken 生成随机令牌
func generateSessionToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// cleanExpiredTokens 清理过期令牌
func cleanExpiredTokens() {
	now := time.Now()
	sessionTokens.Range(func(key, value interface{}) bool {
		tokenInfo := value.(*SessionToken)
		if now.After(tokenInfo.ExpiresAt) {
			sessionTokens.Delete(key)
		}
		return true
	})
}

func (s *SessionService) GetLoginRecords(page, pageSize int, hostID string) ([]model.LoginRecord, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	return s.repo.FindLoginRecords(page, pageSize, hostID)
}

// GetLoginRecordsByUser 获取登录记录（支持按用户过滤，userID为空则返回所有）
func (s *SessionService) GetLoginRecordsByUser(page, pageSize int, hostID, userID string) ([]model.LoginRecordWithType, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	return s.repo.FindLoginRecordsByUser(page, pageSize, hostID, userID)
}

// GetSessionHistories 获取SSH会话历史记录（用于首页展示）
func (s *SessionService) GetSessionHistories(page, pageSize int, hostID string) ([]map[string]interface{}, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	return s.repo.FindSessionHistories(page, pageSize, hostID)
}

func (s *SessionService) GetRecentLogins(limit int, userID string) ([]model.LoginRecord, error) {
	if limit < 1 || limit > 100 {
		limit = 10
	}
	return s.repo.GetRecentLoginsByUser(limit, userID)
}

func (s *SessionService) EndSession(sessionID string) error {
	// 会话结束通过 session_recordings 表管理
	// 登录记录的结束时间和状态由 session_recordings 的回调更新
	// 这里暂时保留方法签名以保持兼容性，实际逻辑在其他地方处理
	return nil
}

func (s *SessionService) GetRecentLoginsCount(hours int, userID string) (int64, error) {
	return s.repo.CountRecentLoginsByUser(hours, userID)
}

// GetTodayLoginsCount 获取今日登录次数（从今天0点开始）
func (s *SessionService) GetTodayLoginsCount(userID string) (int64, error) {
	return s.repo.CountTodayLoginsByUser(userID)
}

// ===== Session Recording Methods =====
// 使用统一的 session_recordings 表（支持 webshell 和 ssh_client）

func (s *SessionService) GetSessionRecordings(page, pageSize int, search string) ([]model.SessionRecording, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 直接从 session_recordings 表查询
	var recordings []model.SessionRecording
	var total int64

	query := s.repo.GetDB().Model(&model.SessionRecording{})

	// 搜索过滤
	if search != "" {
		query = query.Where("session_id LIKE ? OR host_ip LIKE ? OR username LIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("start_time DESC").Find(&recordings).Error; err != nil {
		return nil, 0, err
	}

	// 修复 duration 字段：如果状态是 closed 但 duration 是 "进行中" 或 "???"，重新计算或设置为 "-"
	for i := range recordings {
		if recordings[i].Status == "closed" {
			// 如果 duration 是 "进行中"、"???" 或空，需要修复
			if recordings[i].Duration == "进行中" || recordings[i].Duration == "???" || recordings[i].Duration == "" {
				if recordings[i].EndTime != nil {
					// 有结束时间，重新计算 duration
					diff := recordings[i].EndTime.Sub(recordings[i].StartTime)
					minutes := int(diff.Minutes())
					seconds := int(diff.Seconds()) % 60
					recordings[i].Duration = fmt.Sprintf("%dm %ds", minutes, seconds)
					// 同时更新数据库
					s.repo.GetDB().Model(&model.SessionRecording{}).
						Where("session_id = ?", recordings[i].SessionID).
						Update("duration", recordings[i].Duration)
				} else {
					// 没有结束时间，设置为 "-"
					recordings[i].Duration = "-"
					// 更新数据库
					s.repo.GetDB().Model(&model.SessionRecording{}).
						Where("session_id = ?", recordings[i].SessionID).
						Update("duration", "-")
				}
			}
		}
	}

	// 批量查询主机名（从 hosts 表）
	if len(recordings) > 0 {
		hostIDs := make([]string, 0, len(recordings))
		hostIDSet := make(map[string]bool)
		for _, rec := range recordings {
			if rec.HostID != "" && !hostIDSet[rec.HostID] {
				hostIDs = append(hostIDs, rec.HostID)
				hostIDSet[rec.HostID] = true
			}
		}

		if len(hostIDs) > 0 {
			hostNameMap := make(map[string]string)
			var hosts []model.Host
			s.repo.GetDB().Select("id, name").Where("id IN ?", hostIDs).Find(&hosts)
			for _, host := range hosts {
				hostNameMap[host.ID] = host.Name
			}

			// 更新主机名
			for i := range recordings {
				if recordings[i].HostName == "" || recordings[i].HostName == recordings[i].HostIP {
					if name, ok := hostNameMap[recordings[i].HostID]; ok {
						recordings[i].HostName = name
					}
				}
			}
		}
	}

	// 统计每个会话的命令数（从统一的 command_records 表）
	if len(recordings) > 0 {
		sessionIDs := make([]string, len(recordings))
		for i, rec := range recordings {
			sessionIDs[i] = rec.SessionID
		}

		type CommandCount struct {
			SessionID string
			Count     int
		}
		var counts []CommandCount

		// 统计命令数
		s.repo.GetDB().Model(&model.CommandRecord{}).
			Select("session_id, COUNT(*) as count").
			Where("session_id IN ?", sessionIDs).
			Group("session_id").
			Scan(&counts)

		commandCountMap := make(map[string]int)
		for _, c := range counts {
			commandCountMap[c.SessionID] = c.Count
		}

		// 更新命令数（如果数据库中的 command_count 与实际不符）
		for i := range recordings {
			actualCount := commandCountMap[recordings[i].SessionID]
			recordings[i].CommandCount = actualCount
		}
	}

	return recordings, total, nil
}

func (s *SessionService) GetSessionRecording(sessionID string) (*model.SessionRecording, error) {
	// 直接从 session_recordings 表查询
	var recording model.SessionRecording
	if err := s.repo.GetDB().Where("session_id = ?", sessionID).First(&recording).Error; err != nil {
		return nil, err
	}

	// 查询主机名（如果没有）
	if recording.HostName == "" && recording.HostID != "" {
		var host model.Host
		if err := s.repo.GetDB().Select("name").Where("id = ?", recording.HostID).First(&host).Error; err == nil {
			recording.HostName = host.Name
		}
	}

	// 统计命令数（从统一的 command_records 表）
	var count int64
	s.repo.GetDB().Model(&model.CommandRecord{}).
		Where("session_id = ?", sessionID).
		Count(&count)
	recording.CommandCount = int(count)

	// 修复 duration 字段：如果状态是 closed 但 duration 是 "进行中" 或 "???"，重新计算或设置为 "-"
	if recording.Status == "closed" {
		// 如果 duration 是 "进行中"、"???" 或空，需要修复
		if recording.Duration == "进行中" || recording.Duration == "???" || recording.Duration == "" {
			if recording.EndTime != nil {
				// 有结束时间，重新计算 duration
				diff := recording.EndTime.Sub(recording.StartTime)
				minutes := int(diff.Minutes())
				seconds := int(diff.Seconds()) % 60
				recording.Duration = fmt.Sprintf("%dm %ds", minutes, seconds)
				// 同时更新数据库
				s.repo.GetDB().Model(&model.SessionRecording{}).
					Where("session_id = ?", recording.SessionID).
					Update("duration", recording.Duration)
			} else {
				// 没有结束时间，设置为 "-"
				recording.Duration = "-"
				// 更新数据库
				s.repo.GetDB().Model(&model.SessionRecording{}).
					Where("session_id = ?", recording.SessionID).
					Update("duration", "-")
			}
		}
	}

	return &recording, nil
}

func (s *SessionService) CreateSessionRecording(recording *model.SessionRecording) error {
	recording.ID = uuid.New().String()
	recording.CreatedAt = time.Now()
	recording.UpdatedAt = time.Now()
	return s.repo.CreateSessionRecording(recording)
}

func (s *SessionService) UpdateSessionRecording(sessionID string, updates map[string]interface{}) error {
	return s.repo.UpdateSessionRecording(sessionID, updates)
}

// ===== Command Record Methods =====
// 使用统一的 command_records 表（支持 webshell 和 ssh_client）

func (s *SessionService) GetCommandRecords(page, pageSize int, search, hostFilter string) ([]model.CommandRecord, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 从 command_histories 表查询（CommandRecord 的表名）
	var records []model.CommandRecord
	var total int64

	query := s.repo.GetDB().Model(&model.CommandRecord{})

	// 搜索过滤
	if search != "" {
		query = query.Where("command LIKE ? OR host_ip LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	// 主机筛选
	if hostFilter != "" && hostFilter != "all" {
		query = query.Where("host_ip = ?", hostFilter)
	}

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		log.Printf("[SessionService] ERROR: Failed to count command records: %v", err)
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("executed_at DESC").Find(&records).Error; err != nil {
		log.Printf("[SessionService] ERROR: Failed to query command records: %v", err)
		return nil, 0, err
	}

	return records, total, nil
}

func (s *SessionService) CreateCommandRecord(record *model.CommandRecord) error {
	// ID是自增的，不需要手动设置
	record.CreatedAt = time.Now()
	return s.repo.CreateCommandRecord(record)
}

func (s *SessionService) GetCommandsBySession(sessionID string) ([]model.CommandRecord, error) {
	// 直接从 command_records 表查询指定会话的命令
	var records []model.CommandRecord
	if err := s.repo.GetDB().Where("session_id = ?", sessionID).Order("executed_at ASC").Find(&records).Error; err != nil {
		return nil, err
	}

	return records, nil
}

// TerminateSession 终止会话
func (s *SessionService) TerminateSession(sessionID string) error {
	// 1. 从数据库查询会话信息（统一的 session_recordings 表）
	var recording model.SessionRecording
	if err := s.repo.GetDB().Where("session_id = ?", sessionID).First(&recording).Error; err != nil {
		return fmt.Errorf("会话不存在: %w", err)
	}

	// 2. 检查会话状态
	if recording.Status != "active" {
		return fmt.Errorf("会话已结束，无需终止")
	}

	// 3. 获取 Proxy 信息（仅 webshell 需要）
	if recording.ConnectionType == "webshell" && recording.ProxyID != "" {
		var proxy struct {
			ID      string `gorm:"column:id"`
			ProxyID string `gorm:"column:proxy_id"`
			IP      string `gorm:"column:ip"`
			Port    int    `gorm:"column:port"`
			Status  string `gorm:"column:status"`
		}
		if err := s.repo.GetDB().Table("proxies").
			Where("proxy_id = ?", recording.ProxyID).
			First(&proxy).Error; err != nil {
			log.Printf("[TerminateSession] 无法找到 Proxy 信息: %v", err)
			return fmt.Errorf("无法找到 Proxy 信息: %w", err)
		}

		log.Printf("[TerminateSession] Found proxy: ID=%s, IP=%s, Port=%d", proxy.ProxyID, proxy.IP, proxy.Port)

		// 4. 调用 Proxy 的终止会话 API
		proxyURL := fmt.Sprintf("http://%s:%d/api/sessions/%s/terminate",
			proxy.IP, proxy.Port, sessionID)
		log.Printf("[TerminateSession] Calling proxy URL: %s", proxyURL)

		req, err := http.NewRequest("DELETE", proxyURL, nil)
		if err != nil {
			log.Printf("[TerminateSession] 创建请求失败: %v", err)
			return fmt.Errorf("创建请求失败: %w", err)
		}

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[TerminateSession] 调用 Proxy API 失败: %v", err)
			return fmt.Errorf("调用 Proxy API 失败: %w", err)
		}
		defer resp.Body.Close()

		log.Printf("[TerminateSession] Proxy response status: %d", resp.StatusCode)

		if resp.StatusCode != http.StatusOK {
			log.Printf("[TerminateSession] Proxy 返回错误状态: %d", resp.StatusCode)
			return fmt.Errorf("Proxy 返回错误状态: %d", resp.StatusCode)
		}
	} else {
		log.Printf("[TerminateSession] SSH客户端会话，不需要调用 Proxy API")
	}

	// 5. 更新数据库状态（统一的 session_recordings 表和 login_records）
	now := time.Now()
	diff := now.Sub(recording.StartTime)
	minutes := int(diff.Minutes())
	seconds := int(diff.Seconds()) % 60
	duration := fmt.Sprintf("%dm %ds", minutes, seconds)

	// 5.1 更新 session_recordings
	sessionUpdates := map[string]interface{}{
		"status":   "closed",
		"end_time": now,
		"duration": duration,
	}
	if err := s.repo.GetDB().Model(&model.SessionRecording{}).
		Where("session_id = ?", sessionID).
		Updates(sessionUpdates).Error; err != nil {
		return fmt.Errorf("更新 session_recordings 状态失败: %w", err)
	}

	// 5.2 更新 login_records
	loginUpdates := map[string]interface{}{
		"status":      "closed",
		"logout_time": now,
		"duration":    int(diff.Seconds()),
	}
	if err := s.repo.GetDB().Table("login_records").
		Where("session_id = ?", sessionID).
		Updates(loginUpdates).Error; err != nil {
		// 记录错误但不影响主流程
		log.Printf("[SessionService] 更新 login_records 失败: %v", err)
	}

	return nil
}
