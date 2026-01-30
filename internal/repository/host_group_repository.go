package repository

import (
	"fmt"

	"github.com/fisker/zjump-backend/internal/model"
	"gorm.io/gorm"
)

// HostGroupRepository 主机分组仓库
type HostGroupRepository struct {
	db *gorm.DB
}

// NewHostGroupRepository 创建主机分组仓库
func NewHostGroupRepository(db *gorm.DB) *HostGroupRepository {
	return &HostGroupRepository{db: db}
}

// ============================================================================
// 分组 CRUD
// ============================================================================

// Create 创建分组
func (r *HostGroupRepository) Create(group *model.HostGroup) error {
	return r.db.Create(group).Error
}

// FindByID 根据ID查询分组
func (r *HostGroupRepository) FindByID(id string) (*model.HostGroup, error) {
	var group model.HostGroup
	err := r.db.Where("id = ?", id).First(&group).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// FindByName 根据名称查询分组
func (r *HostGroupRepository) FindByName(name string) (*model.HostGroup, error) {
	var group model.HostGroup
	err := r.db.Where("name = ?", name).First(&group).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// FindAll 查询所有分组
func (r *HostGroupRepository) FindAll() ([]model.HostGroup, error) {
	var groups []model.HostGroup
	err := r.db.Order("sort_order ASC, name ASC").Find(&groups).Error
	return groups, err
}

// FindAllWithStats 查询所有分组并带统计信息
func (r *HostGroupRepository) FindAllWithStats() ([]model.HostGroup, error) {
	var groups []model.HostGroup

	// 查询所有分组
	err := r.db.Order("sort_order ASC, name ASC").Find(&groups).Error
	if err != nil {
		return nil, err
	}

	// 为每个分组添加统计信息
	for i := range groups {
		// 统计主机总数
		var hostCount int64
		r.db.Model(&model.HostGroupMember{}).Where("group_id = ?", groups[i].ID).Count(&hostCount)
		groups[i].HostCount = int(hostCount)

		// 统计在线主机数
		var onlineCount int64
		r.db.Table("host_group_members").
			Joins("JOIN hosts ON hosts.id = host_group_members.host_id").
			Where("host_group_members.group_id = ? AND hosts.status = ?", groups[i].ID, "online").
			Count(&onlineCount)
		groups[i].OnlineCount = int(onlineCount)
	}

	return groups, nil
}

// Update 更新分组
func (r *HostGroupRepository) Update(group *model.HostGroup) error {
	return r.db.Save(group).Error
}

// Delete 删除分组
func (r *HostGroupRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.HostGroup{}).Error
}

// ============================================================================
// 主机-分组关联
// ============================================================================

// AddHostToGroup 添加主机到分组
func (r *HostGroupRepository) AddHostToGroup(groupID, hostID, addedBy string) error {
	member := &model.HostGroupMember{
		GroupID: groupID,
		HostID:  hostID,
		AddedBy: addedBy,
	}
	return r.db.Create(member).Error
}

// RemoveHostFromGroup 从分组移除主机
func (r *HostGroupRepository) RemoveHostFromGroup(groupID, hostID string) error {
	return r.db.Where("group_id = ? AND host_id = ?", groupID, hostID).
		Delete(&model.HostGroupMember{}).Error
}

// AddHostsToGroup 批量添加主机到分组
func (r *HostGroupRepository) AddHostsToGroup(groupID string, hostIDs []string, addedBy string) error {
	// 检查哪些主机已经在分组中
	var existingMembers []model.HostGroupMember
	if err := r.db.Where("group_id = ? AND host_id IN ?", groupID, hostIDs).
		Find(&existingMembers).Error; err != nil {
		return fmt.Errorf("failed to check existing members: %w", err)
	}

	// 构建已存在主机的ID集合
	existingHostIDs := make(map[string]bool)
	for _, member := range existingMembers {
		existingHostIDs[member.HostID] = true
	}

	// 只添加不在分组中的主机
	members := make([]model.HostGroupMember, 0)
	for _, hostID := range hostIDs {
		if !existingHostIDs[hostID] {
			members = append(members, model.HostGroupMember{
				GroupID: groupID,
				HostID:  hostID,
				AddedBy: addedBy,
			})
		}
	}

	// 如果没有需要添加的主机，直接返回成功
	if len(members) == 0 {
		return nil
	}

	return r.db.Create(&members).Error
}

// RemoveHostsFromGroup 批量从分组移除主机
func (r *HostGroupRepository) RemoveHostsFromGroup(groupID string, hostIDs []string) error {
	return r.db.Where("group_id = ? AND host_id IN ?", groupID, hostIDs).
		Delete(&model.HostGroupMember{}).Error
}

// GetHostsByGroupID 获取分组中的所有主机
func (r *HostGroupRepository) GetHostsByGroupID(groupID string) ([]model.Host, error) {
	var hosts []model.Host

	err := r.db.Table("hosts").
		Joins("JOIN host_group_members ON hosts.id = host_group_members.host_id").
		Where("host_group_members.group_id = ?", groupID).
		Order("hosts.name ASC").
		Find(&hosts).Error

	return hosts, err
}

// GetHostsByGroupIDWithPagination 分页获取分组中的主机
func (r *HostGroupRepository) GetHostsByGroupIDWithPagination(groupID string, page, pageSize int) ([]model.Host, int64, error) {
	var hosts []model.Host
	var total int64

	query := r.db.Table("hosts").
		Joins("JOIN host_group_members ON hosts.id = host_group_members.host_id").
		Where("host_group_members.group_id = ?", groupID)

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err := query.Offset(offset).Limit(pageSize).Order("hosts.name ASC").Find(&hosts).Error

	return hosts, total, err
}

// GetGroupsByHostID 获取主机所属的所有分组
func (r *HostGroupRepository) GetGroupsByHostID(hostID string) ([]model.HostGroup, error) {
	var groups []model.HostGroup

	err := r.db.Table("host_groups").
		Joins("JOIN host_group_members ON host_groups.id = host_group_members.group_id").
		Where("host_group_members.host_id = ?", hostID).
		Order("host_groups.sort_order ASC, host_groups.name ASC").
		Find(&groups).Error

	return groups, err
}

// IsHostInGroup 检查主机是否在分组中
func (r *HostGroupRepository) IsHostInGroup(groupID, hostID string) (bool, error) {
	var count int64
	err := r.db.Model(&model.HostGroupMember{}).
		Where("group_id = ? AND host_id = ?", groupID, hostID).
		Count(&count).Error
	return count > 0, err
}

// GetGroupStatistics 获取分组统计信息
func (r *HostGroupRepository) GetGroupStatistics(groupID string) (*model.HostGroupStatistics, error) {
	var stats model.HostGroupStatistics

	// 获取分组名称
	var group model.HostGroup
	if err := r.db.Where("id = ?", groupID).First(&group).Error; err != nil {
		return nil, err
	}
	stats.GroupID = groupID
	stats.GroupName = group.Name

	// 统计总主机数
	var totalHosts int64
	r.db.Model(&model.HostGroupMember{}).Where("group_id = ?", groupID).Count(&totalHosts)
	stats.TotalHosts = int(totalHosts)

	// 统计在线主机数
	var onlineHosts int64
	r.db.Table("host_group_members").
		Joins("JOIN hosts ON hosts.id = host_group_members.host_id").
		Where("host_group_members.group_id = ? AND hosts.status = ?", groupID, "online").
		Count(&onlineHosts)
	stats.OnlineHosts = int(onlineHosts)

	// 计算离线主机数
	stats.OfflineHosts = stats.TotalHosts - stats.OnlineHosts

	return &stats, nil
}

// SearchHostsInGroup 在分组中搜索主机
func (r *HostGroupRepository) SearchHostsInGroup(groupID, keyword string) ([]model.Host, error) {
	var hosts []model.Host

	query := r.db.Table("hosts").
		Joins("JOIN host_group_members ON hosts.id = host_group_members.host_id").
		Where("host_group_members.group_id = ?", groupID)

	if keyword != "" {
		keyword = "%" + keyword + "%"
		query = query.Where("hosts.name LIKE ? OR hosts.ip LIKE ?", keyword, keyword)
	}

	err := query.Order("hosts.name ASC").Find(&hosts).Error
	return hosts, err
}

// GetDB 获取数据库连接（供其他需要的地方使用）
func (r *HostGroupRepository) GetDB() *gorm.DB {
	return r.db
}

// ============================================================================
// 批量操作
// ============================================================================

// BatchUpdateSortOrder 批量更新分组排序
func (r *HostGroupRepository) BatchUpdateSortOrder(updates map[string]int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		for groupID, sortOrder := range updates {
			if err := tx.Model(&model.HostGroup{}).
				Where("id = ?", groupID).
				Update("sort_order", sortOrder).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// MoveHostsBetweenGroups 在分组间移动主机
func (r *HostGroupRepository) MoveHostsBetweenGroups(fromGroupID, toGroupID string, hostIDs []string, movedBy string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 从原分组移除
		if err := tx.Where("group_id = ? AND host_id IN ?", fromGroupID, hostIDs).
			Delete(&model.HostGroupMember{}).Error; err != nil {
			return fmt.Errorf("failed to remove from group: %w", err)
		}

		// 添加到新分组
		members := make([]model.HostGroupMember, 0, len(hostIDs))
		for _, hostID := range hostIDs {
			members = append(members, model.HostGroupMember{
				GroupID: toGroupID,
				HostID:  hostID,
				AddedBy: movedBy,
			})
		}

		if err := tx.Create(&members).Error; err != nil {
			return fmt.Errorf("failed to add to new group: %w", err)
		}

		return nil
	})
}
