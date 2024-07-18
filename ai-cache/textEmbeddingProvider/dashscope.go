package TextEmbeddingProvider

import "errors"

type dashScopeProviderInitializer struct {
}

func (d *dashScopeProviderInitializer) ValidateConfig(config ProviderConfig) error {
	if len(config.DashScopeKey) == 0 {
		return errors.New("DashScopeKey is required")
	}
	if len(config.DashScopeServiceName) == 0 {
		return errors.New("DashScopeServiceName is required")
	}
	return nil
}

func (d *dashScopeProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	return &DSProvider{config: config}, nil
}

type DSProvider struct {
	config ProviderConfig
}

func (d *DSProvider) GetProviderType() string {
	return providerTypeDashScope
}

// TODO: Implement the GetEmbedding method
func (d *DSProvider) GetEmbedding(text string, callback func([]float64, error)) error {
	return nil
}
