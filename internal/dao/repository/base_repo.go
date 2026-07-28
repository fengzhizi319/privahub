// Package repository provides GORM-based implementations of repository interfaces.
package repository

import (
	"context"

	"gorm.io/gorm"
)

// BaseRepo is a generic GORM repository implementation.
type BaseRepo[T any] struct {
	db *gorm.DB
}

// NewBaseRepo creates a new generic repository.
func NewBaseRepo[T any](db *gorm.DB) *BaseRepo[T] {
	return &BaseRepo[T]{db: db}
}

// Create inserts a new entity.
func (r *BaseRepo[T]) Create(ctx context.Context, entity *T) error {
	return r.db.WithContext(ctx).Create(entity).Error
}

// BatchCreate inserts multiple entities.
func (r *BaseRepo[T]) BatchCreate(ctx context.Context, entities []T) error {
	if len(entities) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&entities).Error
}

// Update updates an existing entity.
func (r *BaseRepo[T]) Update(ctx context.Context, entity *T) error {
	return r.db.WithContext(ctx).Save(entity).Error
}

// Delete soft-deletes an entity by ID.
func (r *BaseRepo[T]) Delete(ctx context.Context, id int64) error {
	var entity T
	return r.db.WithContext(ctx).Delete(&entity, id).Error
}

// FindByID retrieves an entity by primary key.
func (r *BaseRepo[T]) FindByID(ctx context.Context, id int64) (*T, error) {
	var entity T
	err := r.db.WithContext(ctx).First(&entity, id).Error
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

// FindAll retrieves all entities.
func (r *BaseRepo[T]) FindAll(ctx context.Context) ([]T, error) {
	var entities []T
	err := r.db.WithContext(ctx).Find(&entities).Error
	return entities, err
}

// DB returns the underlying gorm.DB for custom queries.
func (r *BaseRepo[T]) DB() *gorm.DB {
	return r.db
}
