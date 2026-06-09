package application

import (
	"sync"

	"github.com/raywall/custom-business-metrics/service/internal/core"
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
	return &ConfigService{config: normalizeRuntimeConfig(core.RuntimeConfig{RetentionDays: retentionDays})}
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
	s.config = normalizeRuntimeConfig(config)
	return s.config
}

func normalizeRuntimeConfig(config core.RuntimeConfig) core.RuntimeConfig {
	if config.RetentionDays <= 0 {
		config.RetentionDays = 7
	}
	config.Features.TracingEnabled = true
	config.Features.TraceIndexEnabled = true
	if !config.Security.Redaction.Enabled && len(config.Security.Redaction.Fields) == 0 {
		config.Security.Redaction.Enabled = true
		config.Security.Redaction.Fields = []string{"authorization", "client_secret", "access_token", "refresh_token", "password", "token", "api_key", "x-api-key"}
	}
	return config
}
