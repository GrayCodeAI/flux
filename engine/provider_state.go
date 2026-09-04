package engine

import (
	"path/filepath"
	"sync"

	"github.com/GrayCodeAI/graycode-router/config"
)

var providerStateLocks sync.Map

func lockProviderStatePath(path string) func() {
	key, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		key = filepath.Clean(path)
	}
	value, _ := providerStateLocks.LoadOrStore(key, &sync.Mutex{})
	mu, ok := value.(*sync.Mutex)
	if !ok {
		panic("graycode-router engine: provider-state lock map contains a non-mutex value")
	}
	mu.Lock()
	return mu.Unlock
}

func (e *Engine) loadProviderConfigStrict() (*config.ProviderConfig, error) {
	cfg, err := config.LoadProviderConfigWithError(e.providerConfigPath)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = &config.ProviderConfig{}
	}
	return cfg, nil
}
