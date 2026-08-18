package store

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// StrategyVersion a point-in-time snapshot of a strategy config.
type StrategyVersion struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	StrategyID string    `gorm:"column:strategy_id;not null;index:idx_strategy_versions_strategy" json:"strategy_id"`
	UserID     string    `gorm:"column:user_id;not null;index" json:"-"`
	Version    int       `gorm:"column:version;not null" json:"version"`
	Config     string    `gorm:"column:config;not null" json:"config"`
	Note       string    `gorm:"column:note;default:''" json:"note"`
	IsCurrent  bool      `gorm:"column:is_current;default:false" json:"is_current"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
}

func (StrategyVersion) TableName() string { return "strategy_versions" }

type StrategyVersionStore struct {
	db *gorm.DB
}

func NewStrategyVersionStore(db *gorm.DB) *StrategyVersionStore {
	return &StrategyVersionStore{db: db}
}

func (s *StrategyVersionStore) initTables() error {
	if s.db.Dialector.Name() == "postgres" {
		var exists int64
		s.db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'strategy_versions'`).Scan(&exists)
		if exists > 0 {
			return nil
		}
	}
	return s.db.AutoMigrate(&StrategyVersion{})
}

func (s *StrategyVersionStore) List(strategyID, userID string) ([]*StrategyVersion, error) {
	var versions []*StrategyVersion
	if err := s.db.Where("strategy_id = ? AND user_id = ?", strategyID, userID).
		Order("version ASC").Find(&versions).Error; err != nil {
		return nil, fmt.Errorf("failed to list strategy versions: %w", err)
	}
	return versions, nil
}

func (s *StrategyVersionStore) Get(strategyID, userID string, version int) (*StrategyVersion, error) {
	var v StrategyVersion
	if err := s.db.Where("strategy_id = ? AND user_id = ? AND version = ?", strategyID, userID, version).
		First(&v).Error; err != nil {
		return nil, fmt.Errorf("failed to get strategy version: %w", err)
	}
	return &v, nil
}

func (s *StrategyVersionStore) CreateSnapshot(strategyID, userID, config, note string) (*StrategyVersion, error) {
	var maxVersion *int
	s.db.Model(&StrategyVersion{}).Where("strategy_id = ? AND user_id = ?", strategyID, userID).
		Select("COALESCE(MAX(version), 0)").Scan(&maxVersion)
	next := 1
	if maxVersion != nil {
		next = *maxVersion + 1
	}
	v := &StrategyVersion{
		StrategyID: strategyID,
		UserID:     userID,
		Version:    next,
		Config:     config,
		Note:       note,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.db.Create(v).Error; err != nil {
		return nil, fmt.Errorf("failed to create strategy version: %w", err)
	}
	return v, nil
}

func (s *StrategyVersionStore) SetCurrent(strategyID, userID string, version int) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&StrategyVersion{}).Where("strategy_id = ? AND user_id = ?", strategyID, userID).
			Update("is_current", false).Error; err != nil {
			return err
		}
		return tx.Model(&StrategyVersion{}).Where("strategy_id = ? AND user_id = ? AND version = ?", strategyID, userID, version).
			Update("is_current", true).Error
	})
}

func (s *StrategyVersionStore) DeleteForStrategy(strategyID, userID string) error {
	return s.db.Where("strategy_id = ? AND user_id = ?", strategyID, userID).
		Delete(&StrategyVersion{}).Error
}
