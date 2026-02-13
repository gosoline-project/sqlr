package sqlr

import "time"

type KeyTypes interface {
	bool | string | int | int64 | uint | uint64 | float32 | float64 |
		*bool | *string | *int | *int64 | *uint | *uint64 | *float32 | *float64
}

type Entitier[K KeyTypes] interface {
	GetId() K
	GetUpdatedAt() time.Time
	GetCreatedAt() time.Time
}

var _ Entitier[string] = (*Entity[string])(nil)

type Entity[K KeyTypes] struct {
	Id        K         `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"autoCreateTime:true"`
	UpdatedAt time.Time `gorm:"autoUpdateTime:true"`
}

func (e Entity[K]) GetId() K {
	return e.Id
}

func (e Entity[K]) GetUpdatedAt() time.Time {
	return e.UpdatedAt
}

func (e Entity[K]) GetCreatedAt() time.Time {
	return e.CreatedAt
}
