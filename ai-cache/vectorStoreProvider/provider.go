package vectorStorePrvider

import (
	"github.com/alibaba/higress/plugins/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	providerTypeDashVector = "dashvector"
)

type providerInitializer interface {
	ValidateConfig(ProviderConfig) error
	CreateProvider(ProviderConfig) (Provider, error)
}

var (
	providerInitializers = map[string]providerInitializer{
		providerTypeDashVector: &dashVectorProviderInitializer{},
	}
)

type ProviderConfig struct {
	// @Title zh-CN 向量存储服务提供者类型
	// @Description zh-CN 向量存储服务提供者类型，例如 DashVector、Milvus
	typ string `json:"vectorStoreProviderType"`
	// @Title zh-CN DashVector 阿里云向量搜索引擎
	// @Description zh-CN 调用阿里云的向量搜索引擎
	DashVectorServiceName string `require:"true" yaml:"DashVectorServiceName" jaon:"DashVectorServiceName"`
	// @Title zh-CN DashVector Key
	// @Description zh-CN 阿里云向量搜索引擎的 key
	DashVectorKey string `require:"true" yaml:"DashVectorKey" jaon:"DashVectorKey"`
	// @Title zh-CN DashVector AuthApiEnd
	// @Description zh-CN 阿里云向量搜索引擎的 AuthApiEnd
	DashVectorAuthApiEnd string `require:"true" yaml:"DashVectorEnd" jaon:"DashVectorEnd"`
	// @Title zh-CN DashVector Collection
	// @Description zh-CN 指定使用阿里云搜索引擎中的哪个向量集合
	DashVectorCollection string `require:"true" yaml:"DashVectorCollection" jaon:"DashVectorCollection"`
	// @Title zh-CN DashVector Client
	// @Description zh-CN 阿里云向量搜索引擎的 Client
	DashVectorClient wrapper.HttpClient `yaml:"-" json:"-"`
}

type Provider interface {
	GetProviderType() string
}

type QueryEmbedding interface {
	QueryEmbedding(req QueryRequest, callback func(resp QueryResponse, err error)) error
}

func (c *ProviderConfig) FromJson(json gjson.Result) {
	c.typ = json.Get("vectorStoreProviderType").String()

}

// QueryResponse 定义查询响应的结构
type QueryResponse struct {
	Code      int      `json:"code"`
	RequestID string   `json:"request_id"`
	Message   string   `json:"message"`
	Output    []Result `json:"output"`
}

// QueryRequest 定义查询请求的结构
type QueryRequest struct {
	Vector        []float64 `json:"vector"`
	TopK          int       `json:"topk"`
	IncludeVector bool      `json:"include_vector"`
}

// Result 定义查询结果的结构
type Result struct {
	ID     string                 `json:"id"`
	Vector []float64              `json:"vector,omitempty"` // omitempty 使得如果 vector 是空，它将不会被序列化
	Fields map[string]interface{} `json:"fields"`
	Score  float64                `json:"score"`
}
