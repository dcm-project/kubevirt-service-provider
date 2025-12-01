package store

import (
	"context"

	"github.com/dcm-project/kubevirt-service-provider/internal/store/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProviderApplication interface {
	List(ctx context.Context) (model.ProviderApplicationList, error)
	Create(ctx context.Context, app model.ProviderApplication) (*model.ProviderApplication, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Get(ctx context.Context, id uuid.UUID) (*model.ProviderApplication, error)
	Update(ctx context.Context, app model.ProviderApplication) error
}

type ProviderApplicationStore struct {
	db *gorm.DB
}

var _ ProviderApplication = (*ProviderApplicationStore)(nil)

func NewProviderApplication(db *gorm.DB) ProviderApplication {
	return &ProviderApplicationStore{db: db}
}

func (s *ProviderApplicationStore) List(ctx context.Context) (model.ProviderApplicationList, error) {
	var apps model.ProviderApplicationList
	tx := s.db.WithContext(ctx).Model(&apps)
	result := tx.Find(&apps)
	if result.Error != nil {
		return nil, result.Error
	}
	return apps, nil
}

func (s *ProviderApplicationStore) Delete(ctx context.Context, id uuid.UUID) error {
	result := s.db.WithContext(ctx).Delete(&model.ProviderApplication{}, id)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *ProviderApplicationStore) Create(ctx context.Context, app model.ProviderApplication) (*model.ProviderApplication, error) {
	result := s.db.WithContext(ctx).Clauses(clause.Returning{}).Create(&app)
	if result.Error != nil {
		return nil, result.Error
	}

	return &app, nil
}

func (s *ProviderApplicationStore) Get(ctx context.Context, id uuid.UUID) (*model.ProviderApplication, error) {
	var app model.ProviderApplication
	result := s.db.WithContext(ctx).First(&app, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &app, nil
}

func (s *ProviderApplicationStore) Update(ctx context.Context, app model.ProviderApplication) error {
	result := s.db.WithContext(ctx).Model(&model.ProviderApplication{}).Where("id = ?", app.ID).Updates(app)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
