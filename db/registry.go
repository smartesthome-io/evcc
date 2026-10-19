package db

import (
	"slices"
	"sync"

	"gorm.io/gorm"
)

var (
	mu       sync.Mutex
	registry []func(db *gorm.DB) error
	models   []any
)

func Register(fun func(db *gorm.DB) error) {
	mu.Lock()
	defer mu.Unlock()
	registry = append(registry, fun)
}

// RegisterModels registers persisted models for copying between databases.
// Models must be pointers to structs, referenced tables come first.
func RegisterModels(m ...any) {
	mu.Lock()
	defer mu.Unlock()
	models = append(models, m...)
}

// Models returns the registered models
func Models() []any {
	mu.Lock()
	defer mu.Unlock()
	return slices.Clone(models)
}
