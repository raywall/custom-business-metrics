package application

import (
	"sync"

	"custom-business-metrics/service/internal/core"
)

// ConfigService stores runtime configuration for the MVP.
type ConfigService struct {
	mu     sync.RWMutex
	config core.RuntimeConfig
}

// NewConfigService creates a runtime configuration service.
func NewConfigService(retentionDays int) *ConfigService {
	if retentionDays <= 0 {
		retentionDays = 7
	}
	return &ConfigService{config: core.RuntimeConfig{RetentionDays: retentionDays}}
}

// Get returns current runtime configuration.
func (s *ConfigService) Get() core.RuntimeConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// Save updates runtime configuration.
func (s *ConfigService) Save(config core.RuntimeConfig) core.RuntimeConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	if config.RetentionDays <= 0 {
		config.RetentionDays = 1
	}
	s.config = config
	return s.config
}
