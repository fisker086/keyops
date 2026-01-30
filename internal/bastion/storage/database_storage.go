package storage

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/fisker/zjump-backend/internal/model"
	"gorm.io/gorm"
)

// DatabaseStorage 直接写入数据库的存储实现（异步）
// 用于 API Server 在直连模式下的会话和命令记录
type DatabaseStorage struct {
	db           *gorm.DB
	sessionQueue chan *SessionRecord
	commandQueue chan *CommandRecord
	closeQueue   chan *closeRequest
	wg           sync.WaitGroup
	closed       bool
	closedMu     sync.Mutex
}

// closeRequest 关闭会话请求
type closeRequest struct {
	sessionID string
	recording string
}

// NewDatabaseStorage 创建数据库存储（带异步处理）
func NewDatabaseStorage(db *gorm.DB) *DatabaseStorage {
	storage := &DatabaseStorage{
		db:           db,
		sessionQueue: make(chan *SessionRecord, 100),  // 缓冲100个会话
		commandQueue: make(chan *CommandRecord, 1000), // 缓冲1000个命令
		closeQueue:   make(chan *closeRequest, 100),   // 缓冲100个关闭请求
		closed:       false,
	}

	// 启动异步处理 goroutines
	storage.wg.Add(3)
	go storage.processSessionQueue()
	go storage.processCommandQueue()
	go storage.processCloseQueue()

	log.Println("[DatabaseStorage] Async storage initialized with 3 worker goroutines")

	return storage
}

// processSessionQueue 异步处理会话队列
func (s *DatabaseStorage) processSessionQueue() {
	defer s.wg.Done()

	for session := range s.sessionQueue {
		if err := s.saveSessionToDB(session); err != nil {
			log.Printf("[DatabaseStorage] Failed to save session %s: %v", session.SessionID, err)
		}
	}
	log.Println("[DatabaseStorage] Session queue processor stopped")
}

// processCommandQueue 异步处理命令队列
func (s *DatabaseStorage) processCommandQueue() {
	defer s.wg.Done()

	for cmd := range s.commandQueue {
		if err := s.saveCommandToDB(cmd); err != nil {
			log.Printf("[DatabaseStorage] Failed to save command %s: %v", cmd.Command, err)
		}
	}
	log.Println("[DatabaseStorage] Command queue processor stopped")
}

// processCloseQueue 异步处理关闭请求队列
func (s *DatabaseStorage) processCloseQueue() {
	defer s.wg.Done()

	for req := range s.closeQueue {
		if err := s.closeSessionInDB(req.sessionID, req.recording); err != nil {
			log.Printf("[DatabaseStorage] Failed to close session %s: %v", req.sessionID, err)
		}
	}
	log.Println("[DatabaseStorage] Close queue processor stopped")
}

// SaveSession 保存会话记录（异步）
func (s *DatabaseStorage) SaveSession(session *SessionRecord) error {
	s.closedMu.Lock()
	if s.closed {
		s.closedMu.Unlock()
		return nil
	}
	s.closedMu.Unlock()

	// 非阻塞写入队列，如果队列满了则记录警告但不阻塞
	select {
	case s.sessionQueue <- session:
		// 成功加入队列
		return nil
	default:
		log.Printf("[DatabaseStorage]   Session queue full, dropping session %s", session.SessionID)
		// 队列满了，尝试同步保存（降级方案）
		return s.saveSessionToDB(session)
	}
}

// saveSessionToDB 实际的数据库保存逻辑
func (s *DatabaseStorage) saveSessionToDB(session *SessionRecord) error {
	// 判断连接类型：如果有 proxy_id 说明是 webshell，否则是 ssh_client
	connectionType := "ssh_client"
	if session.ProxyID != "" && session.ProxyID != "api-server-direct" {
		connectionType = "webshell"
	}

	// 计算持续时间
	var duration string
	if session.EndTime != nil {
		diff := session.EndTime.Sub(session.StartTime)
		minutes := int(diff.Minutes())
		seconds := int(diff.Seconds()) % 60
		duration = fmt.Sprintf("%dm %ds", minutes, seconds)
	} else {
		duration = "进行中"
	}

	// 创建统一的会话录制记录
	sessionRecording := model.SessionRecording{
		ID:             session.SessionID,
		SessionID:      session.SessionID,
		ConnectionType: connectionType,
		ProxyID:        session.ProxyID,
		UserID:         session.UserID,
		HostID:         session.HostID,
		HostIP:         session.HostIP,
		Username:       session.Username,
		Status:         session.Status,
		StartTime:      session.StartTime,
		Duration:       duration,
		TerminalCols:   session.TerminalCols,
		TerminalRows:   session.TerminalRows,
	}

	// 如果有结束时间和录制内容，也保存
	if session.EndTime != nil {
		sessionRecording.EndTime = session.EndTime
	}
	if session.Recording != "" {
		sessionRecording.Recording = session.Recording
	}

	// 保存到数据库（使用 FirstOrCreate 避免重复）
	result := s.db.Where("session_id = ?", sessionRecording.SessionID).FirstOrCreate(&sessionRecording)
	if result.Error != nil {
		return result.Error
	}

	// 注意：登录记录由 handler 层管理，不在这里创建
	// 登录记录会在连接尝试时创建（status: connecting），
	// 然后根据连接结果更新状态（active/failed/completed）

	log.Printf("[DatabaseStorage]  Session recording saved: %s (status: %s)", sessionRecording.SessionID, sessionRecording.Status)
	return nil
}

// CloseSession 关闭会话（异步）
func (s *DatabaseStorage) CloseSession(sessionID string, recording string) error {
	s.closedMu.Lock()
	if s.closed {
		s.closedMu.Unlock()
		return nil
	}
	s.closedMu.Unlock()

	req := &closeRequest{
		sessionID: sessionID,
		recording: recording,
	}

	// 非阻塞写入队列
	select {
	case s.closeQueue <- req:
		return nil
	default:
		log.Printf("[DatabaseStorage]   Close queue full, processing synchronously for %s", sessionID)
		// 队列满了，同步处理（降级方案）
		return s.closeSessionInDB(sessionID, recording)
	}
}

// closeSessionInDB 实际的关闭会话逻辑
func (s *DatabaseStorage) closeSessionInDB(sessionID string, recording string) error {
	now := time.Now()

	// 先查询会话的开始时间，用于计算持续时间
	var sessionRec model.SessionRecording
	if err := s.db.Where("session_id = ?", sessionID).First(&sessionRec).Error; err != nil {
		log.Printf("[DatabaseStorage] Session not found: %s, error: %v", sessionID, err)
		return err
	}

	// 计算持续时间
	diff := now.Sub(sessionRec.StartTime)
	minutes := int(diff.Minutes())
	seconds := int(diff.Seconds()) % 60
	duration := fmt.Sprintf("%dm %ds", minutes, seconds)

	// 更新统一的会话录制记录（session_recordings 表）
	result := s.db.Model(&model.SessionRecording{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":    "closed",
			"end_time":  now,
			"duration":  duration,
			"recording": recording,
		})

	if result.Error != nil {
		return result.Error
	}

	log.Printf("[DatabaseStorage] Session closed: %s (duration: %s, recording size: %d bytes)",
		sessionID, duration, len(recording))

	// 更新登录记录（仅适用于 webshell 登录，SSH Gateway 登录不在此表）
	var loginRecord model.LoginRecord
	err := s.db.Where("session_id = ?", sessionID).First(&loginRecord).Error
	if err == nil {
		// 找到了记录，更新它
		durationSec := int(now.Sub(loginRecord.LoginTime).Seconds())
		s.db.Model(&loginRecord).Updates(map[string]interface{}{
			"logout_time": now,
			"status":      "completed",
			"duration":    durationSec,
		})
		log.Printf("[DatabaseStorage] Updated login_record for session: %s", sessionID)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// 如果是其他错误（非记录不存在），记录日志
		log.Printf("[DatabaseStorage] Warning: failed to query login_record for session %s: %v", sessionID, err)
	}
	// 如果是 ErrRecordNotFound，忽略（SSH Gateway 登录不需要 login_record）

	return nil
}

// MarkSessionFailed 标记会话为失败状态
func (s *DatabaseStorage) MarkSessionFailed(sessionID string, reason string) error {
	// 更新会话录制状态
	s.db.Model(&model.SessionRecording{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status": "failed",
		})

	// 更新登录记录状态
	s.db.Model(&model.LoginRecord{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status": "failed",
		})

	log.Printf("[DatabaseStorage] Session marked as failed: %s (reason: %s)", sessionID, reason)
	return nil
}

// SaveLoginRecord 保存登录记录（同步）
func (s *DatabaseStorage) SaveLoginRecord(record *LoginRecord) error {
	loginRecord := model.LoginRecord{
		ID:        record.SessionID + "-login",
		UserID:    record.UserID,
		HostID:    record.HostID,
		HostName:  record.HostName,
		HostIP:    record.HostIP,
		Username:  record.Username,
		LoginTime: record.LoginTime,
		Status:    record.Status,
		SessionID: record.SessionID,
	}

	if err := s.db.Create(&loginRecord).Error; err != nil {
		log.Printf("[DatabaseStorage] Failed to save login record: %v", err)
		return err
	}

	log.Printf("[DatabaseStorage] Login record saved: %s (status: %s)", record.SessionID, record.Status)
	return nil
}

// UpdateLoginRecordStatus 更新登录记录状态（同步）
func (s *DatabaseStorage) UpdateLoginRecordStatus(sessionID string, status string, logoutTime time.Time) error {
	updates := map[string]interface{}{
		"status": status,
	}

	// 如果提供了登出时间，添加到更新中
	if !logoutTime.IsZero() {
		updates["logout_time"] = logoutTime

		// 计算持续时间
		var loginRecord model.LoginRecord
		if err := s.db.Where("session_id = ?", sessionID).First(&loginRecord).Error; err == nil {
			duration := int(logoutTime.Sub(loginRecord.LoginTime).Seconds())
			updates["duration"] = duration
		}
	}

	if err := s.db.Model(&model.LoginRecord{}).
		Where("session_id = ?", sessionID).
		Updates(updates).Error; err != nil {
		log.Printf("[DatabaseStorage] Failed to update login record status: %v", err)
		return err
	}

	log.Printf("[DatabaseStorage] Login record status updated: %s -> %s", sessionID, status)
	return nil
}

// SaveCommand 保存命令记录（异步）
func (s *DatabaseStorage) SaveCommand(cmd *CommandRecord) error {
	s.closedMu.Lock()
	if s.closed {
		s.closedMu.Unlock()
		return nil
	}
	s.closedMu.Unlock()

	// 非阻塞写入队列
	select {
	case s.commandQueue <- cmd:
		return nil
	default:
		log.Printf("[DatabaseStorage]   Command queue full, dropping command: %s", cmd.Command)
		// 队列满了，尝试同步保存（降级方案）
		return s.saveCommandToDB(cmd)
	}
}

// saveCommandToDB 实际的命令保存逻辑
func (s *DatabaseStorage) saveCommandToDB(cmd *CommandRecord) error {
	log.Printf("[DatabaseStorage] Saving command: session=%s, host=%s, user=%s, command=%q", 
		cmd.SessionID, cmd.HostIP, cmd.Username, cmd.Command)
	
	// 使用统一的 CommandRecord 模型（表名是 command_histories）
	commandRecord := model.CommandRecord{
		ProxyID:    cmd.ProxyID,
		SessionID:  cmd.SessionID,
		HostID:     cmd.HostID,
		UserID:     cmd.UserID,
		Username:   cmd.Username,
		HostIP:     cmd.HostIP,
		Command:    cmd.Command,
		Output:     cmd.Output,
		ExitCode:   cmd.ExitCode,
		ExecutedAt: cmd.ExecutedAt,
		DurationMs: cmd.DurationMs,
	}

	log.Printf("[DatabaseStorage] Command record prepared: %+v", commandRecord)

	// 保存到数据库
	if err := s.db.Create(&commandRecord).Error; err != nil {
		log.Printf("[DatabaseStorage] Failed to save command %q: %v", cmd.Command, err)
		return err
	}

	log.Printf("[DatabaseStorage] Command saved successfully: id=%d, command=%q, session=%s", 
		commandRecord.ID, cmd.Command, cmd.SessionID)
	return nil
}

// Close closes the storage and waits for all async tasks to complete
func (s *DatabaseStorage) Close() error {
	s.closedMu.Lock()
	if s.closed {
		s.closedMu.Unlock()
		return nil
	}
	s.closed = true
	s.closedMu.Unlock()

	log.Println("[DatabaseStorage] Closing, waiting for queues to drain...")

	// 关闭所有队列
	close(s.sessionQueue)
	close(s.commandQueue)
	close(s.closeQueue)

	// 等待所有 goroutine 完成
	s.wg.Wait()

	log.Println("[DatabaseStorage] All queues drained, storage closed")
	return nil
}
