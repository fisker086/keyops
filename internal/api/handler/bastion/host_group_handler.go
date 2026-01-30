package bastion

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/fisker/zjump-backend/internal/model"
	"github.com/fisker/zjump-backend/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HostGroupHandler 主机分组处理器
type HostGroupHandler struct {
	groupRepo *repository.HostGroupRepository
	hostRepo  *repository.HostRepository
	userRepo  *repository.UserRepository
}

// NewHostGroupHandler 创建主机分组处理器
func NewHostGroupHandler(groupRepo *repository.HostGroupRepository, hostRepo *repository.HostRepository, userRepo *repository.UserRepository) *HostGroupHandler {
	return &HostGroupHandler{
		groupRepo: groupRepo,
		hostRepo:  hostRepo,
		userRepo:  userRepo,
	}
}

// ============================================================================
// 分组管理 API
// ============================================================================

// ListGroups 获取所有分组
// GET /api/host-groups
func (h *HostGroupHandler) ListGroups(c *gin.Context) {
	includeStats := c.Query("stats") == "true"

	var groups []model.HostGroup
	var err error

	if includeStats {
		groups, err = h.groupRepo.FindAllWithStats()
	} else {
		groups, err = h.groupRepo.FindAll()
	}

	if err != nil {
		log.Printf("[HostGroupHandler] Failed to list groups: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": -1,
			"msg":  "Failed to list groups",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"groups": groups,
			"total":  len(groups),
		},
		"msg": "success",
	})
}

// GetGroup 获取单个分组详情
// GET /api/host-groups/:id
func (h *HostGroupHandler) GetGroup(c *gin.Context) {
	groupID := c.Param("id")

	group, err := h.groupRepo.FindByID(groupID)
	if err != nil {
		log.Printf("[HostGroupHandler] Failed to find group %s: %v", groupID, err)
		c.JSON(http.StatusNotFound, gin.H{
			"code": -1,
			"msg":  "Group not found",
		})
		return
	}

	// 获取统计信息
	stats, _ := h.groupRepo.GetGroupStatistics(groupID)
	if stats != nil {
		group.HostCount = stats.TotalHosts
		group.OnlineCount = stats.OnlineHosts
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": group,
		"msg":  "success",
	})
}

// CreateGroup 创建分组
// POST /api/host-groups
func (h *HostGroupHandler) CreateGroup(c *gin.Context) {
	var group model.HostGroup
	if err := c.ShouldBindJSON(&group); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": -1,
			"msg":  "Invalid request body",
		})
		return
	}

	// 生成ID
	group.ID = uuid.New().String()

	// 获取当前用户ID（从认证中间件）
	if userID, exists := c.Get("userID"); exists {
		group.CreatedBy = userID.(string)
	}

	if err := h.groupRepo.Create(&group); err != nil {
		log.Printf("[HostGroupHandler] Failed to create group: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": -1,
			"msg":  "Failed to create group",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": group,
		"msg":  "success",
	})
}

// UpdateGroup 更新分组
// PUT /api/host-groups/:id
func (h *HostGroupHandler) UpdateGroup(c *gin.Context) {
	groupID := c.Param("id")

	var updates model.HostGroup
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": -1,
			"msg":  "Invalid request body",
		})
		return
	}

	// 检查分组是否存在
	existingGroup, err := h.groupRepo.FindByID(groupID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": -1,
			"msg":  "Group not found",
		})
		return
	}

	// 验证必填字段
	if updates.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": -1,
			"msg":  "Group name is required",
		})
		return
	}

	// 更新字段（只更新提供的字段）
	if updates.Name != "" {
		existingGroup.Name = updates.Name
	}
	if updates.Description != "" || updates.Description == "" {
		existingGroup.Description = updates.Description
	}
	if updates.Color != "" {
		existingGroup.Color = updates.Color
	}
	if updates.Icon != "" {
		existingGroup.Icon = updates.Icon
	}
	// SortOrder 如果是0也可能是有效值，所以检查是否在请求中提供了
	if updates.SortOrder != existingGroup.SortOrder {
		existingGroup.SortOrder = updates.SortOrder
	}

	if err := h.groupRepo.Update(existingGroup); err != nil {
		log.Printf("[HostGroupHandler] Failed to update group: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": -1,
			"msg":  fmt.Sprintf("Failed to update group: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": existingGroup,
		"msg":  "success",
	})
}

// DeleteGroup 删除分组
// DELETE /api/host-groups/:id
func (h *HostGroupHandler) DeleteGroup(c *gin.Context) {
	groupID := c.Param("id")

	if err := h.groupRepo.Delete(groupID); err != nil {
		log.Printf("[HostGroupHandler] Failed to delete group: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": -1,
			"msg":  "Failed to delete group",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
	})
}

// ============================================================================
// 分组主机管理 API
// ============================================================================

// GetGroupHosts 获取分组中的主机列表
// GET /api/host-groups/:id/hosts
func (h *HostGroupHandler) GetGroupHosts(c *gin.Context) {
	groupID := c.Param("id")

	// 获取查询参数
	page := 1
	pageSize := 20
	search := c.Query("search")

	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if ps := c.Query("pageSize"); ps != "" {
		if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 {
			pageSize = parsed
		}
	}

	var hosts []model.Host
	var total int64
	var err error

	if search != "" {
		// 搜索模式
		hosts, err = h.groupRepo.SearchHostsInGroup(groupID, search)
		total = int64(len(hosts))

		// 手动分页
		start := (page - 1) * pageSize
		end := start + pageSize
		if start < len(hosts) {
			if end > len(hosts) {
				end = len(hosts)
			}
			hosts = hosts[start:end]
		} else {
			hosts = []model.Host{}
		}
	} else {
		// 普通分页查询
		hosts, total, err = h.groupRepo.GetHostsByGroupIDWithPagination(groupID, page, pageSize)
	}

	if err != nil {
		log.Printf("[HostGroupHandler] Failed to get hosts in group %s: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": -1,
			"msg":  "Failed to get hosts",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"hosts":    hosts,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
		"msg": "success",
	})
}

// AddHostsToGroup 添加主机到分组
// POST /api/host-groups/:id/hosts
func (h *HostGroupHandler) AddHostsToGroup(c *gin.Context) {
	groupID := c.Param("id")

	var req struct {
		HostIDs []string `json:"hostIds" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": -1,
			"msg":  "Invalid request body",
		})
		return
	}

	// 检查分组是否存在
	if _, err := h.groupRepo.FindByID(groupID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": -1,
			"msg":  "Group not found",
		})
		return
	}

	// 获取当前用户ID
	addedBy := "system"
	if userID, exists := c.Get("userID"); exists {
		addedBy = userID.(string)
	}

	if err := h.groupRepo.AddHostsToGroup(groupID, req.HostIDs, addedBy); err != nil {
		log.Printf("[HostGroupHandler] Failed to add hosts to group: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": -1,
			"msg":  fmt.Sprintf("Failed to add hosts to group: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
	})
}

// RemoveHostsFromGroup 从分组移除主机
// DELETE /api/host-groups/:id/hosts
func (h *HostGroupHandler) RemoveHostsFromGroup(c *gin.Context) {
	groupID := c.Param("id")

	var req struct {
		HostIDs []string `json:"hostIds" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": -1,
			"msg":  "Invalid request body",
		})
		return
	}

	// 检查分组是否存在
	if _, err := h.groupRepo.FindByID(groupID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": -1,
			"msg":  "Group not found",
		})
		return
	}

	if err := h.groupRepo.RemoveHostsFromGroup(groupID, req.HostIDs); err != nil {
		log.Printf("[HostGroupHandler] Failed to remove hosts from group: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": -1,
			"msg":  fmt.Sprintf("Failed to remove hosts from group: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
	})
}

// GetHostGroups 获取主机所属的所有分组
// GET /api/hosts/:id/groups
func (h *HostGroupHandler) GetHostGroups(c *gin.Context) {
	hostID := c.Param("id")

	groups, err := h.groupRepo.GetGroupsByHostID(hostID)
	if err != nil {
		log.Printf("[HostGroupHandler] Failed to get groups for host %s: %v", hostID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": -1,
			"msg":  "Failed to get groups",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": groups,
		"msg":  "success",
	})
}

// GetGroupStatistics 获取分组统计信息
// GET /api/host-groups/:id/statistics
func (h *HostGroupHandler) GetGroupStatistics(c *gin.Context) {
	groupID := c.Param("id")

	stats, err := h.groupRepo.GetGroupStatistics(groupID)
	if err != nil {
		log.Printf("[HostGroupHandler] Failed to get statistics for group %s: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": -1,
			"msg":  "Failed to get statistics",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": stats,
		"msg":  "success",
	})
}

// GetGroupUsers 获取分组的授权用户列表
// GET /api/host-groups/:id/users
func (h *HostGroupHandler) GetGroupUsers(c *gin.Context) {
	groupID := c.Param("id")

	users, err := h.userRepo.GetUsersInGroup(groupID)
	if err != nil {
		log.Printf("[HostGroupHandler] Failed to get users for group %s: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": -1,
			"msg":  "Failed to get users",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"users": users,
		},
		"msg": "success",
	})
}
