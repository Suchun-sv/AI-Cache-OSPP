package TextEmbeddingProvider

import (
	"github.com/tidwall/gjson"
)

const (
	providerTypeDashScope = "dashscope"
)

type providerInitializer interface {
	ValidateConfig(ProviderConfig) error
	CreateProvider(ProviderConfig) (Provider, error)
}

var (
	providerInitializers = map[string]providerInitializer{
		providerTypeDashScope: &dashScopeProviderInitializer{},
	}
)

type ProviderConfig struct {
	// @Title zh-CN 文本特征提取服务提供者类型
	// @Description zh-CN 文本特征提取服务提供者类型，例如 DashScope
	typ string `json:"TextEmbeddingProviderType"`
	// @Title zh-CN DashScope 阿里云大模型服务名
	// @Description zh-CN 调用阿里云的大模型服务
	DashScopeServiceName string `require:"true" yaml:"DashScopeServiceName" jaon:"DashScopeServiceName"`
	DashScopeKey         string `require:"true" yaml:"DashScopeKey" jaon:"DashScopeKey"`
}

type Provider interface {
	GetProviderType() string
}

type GetEmbedding interface {
	GetEmbedding(text string, callback func([]float64, error)) error
}

func (c *ProviderConfig) FromJson(json gjson.Result) {
	c.typ = json.Get("TextEmbeddingProviderType").String()

}
