package store

import (
	"gorm.io/gorm"
)

type Store interface {
	Close() error
	Application() ProviderApplication
}

type DataStore struct {
	db          *gorm.DB
	application ProviderApplication
}

func NewStore(db *gorm.DB) Store {
	return &DataStore{
		db:          db,
		application: NewProviderApplication(db),
	}
}

func (s *DataStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *DataStore) Application() ProviderApplication {
	return s.application
}
