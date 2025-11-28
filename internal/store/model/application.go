package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProviderApplication struct {
	gorm.Model
	ID           uuid.UUID `gorm:"primaryKey;"`
	OsImage      string    `gorm:"not null;"`
	Ram          int       `gorm:"not null;"`
	Cpu          int       `gorm:"not null;"`
	Namespace    string    `gorm:"not null;"`
	VMName       string    `gorm:"not null;"`
	Architecture string    `gorm:"not null;"`
	HostName     string    `gorm:"not null;"`
	Status       string    `gorm:"not null;"`
}

type ProviderApplicationList []ProviderApplication
