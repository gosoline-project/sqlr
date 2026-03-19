package sqlr

import "time"

// KeyTypes defines supported primary key scalar and pointer key types.
type KeyTypes interface {
	bool | string | int | int64 | uint | uint64 | float32 | float64 |
		*bool | *string | *int | *int64 | *uint | *uint64 | *float32 | *float64
}

type Entitier[K KeyTypes] interface {
	GetId() K
	GetUpdatedAt() time.Time
	GetCreatedAt() time.Time
}

// setIdAware is an internal interface for entities that can have their ID set.
// This is satisfied by *Entity[K] and any types embedding Entity[K].
type setIdAware[K KeyTypes] interface {
	SetId(K)
}

var _ Entitier[string] = (*Entity[string])(nil)

type Entity[K KeyTypes] struct {
	Id        K         `db:"id" sqlr:"primaryKey"`
	CreatedAt time.Time `db:"created_at" sqlr:"autoCreateTime"`
	UpdatedAt time.Time `db:"updated_at" sqlr:"autoUpdateTime"`
}

func (e *Entity[K]) SetId(id K) {
	e.Id = id
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
