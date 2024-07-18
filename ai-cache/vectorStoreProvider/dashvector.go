package vectorStorePrvider

import "errors"

type dashVectorProviderInitializer struct {
}

func (d *dashVectorProviderInitializer) ValidateConfig(config ProviderConfig) error {
	if len(config.DashVectorKey) == 0 {
		return errors.New("DashVectorKey is required")
	}
	if len(config.DashVectorAuthApiEnd) == 0 {
		return errors.New("DashVectorEnd is required")
	}
	if len(config.DashVectorCollection) == 0 {
		return errors.New("DashVectorCollection is required")
	}
	if len(config.DashVectorServiceName) == 0 {
		return errors.New("DashVectorServiceName is required")
	}
	return nil
}

func (d *dashVectorProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	return &DvProvider{config: config}, nil
}

type DvProvider struct {
	config ProviderConfig
}

func (d *DvProvider) GetProviderType() string {
	return providerTypeDashVector
}

// TODO: Implement the QueryEmbedding method
func (d *DvProvider) QueryEmbedding(req QueryRequest, callback func(resp QueryResponse, err error)) error {
	return nil
}
