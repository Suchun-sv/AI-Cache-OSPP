// TODO: 在这里写缓存的具体逻辑, 将textEmbeddingPrvider和vectorStoreProvider作为逻辑中的一个函数调用
package main

import (
	"fmt"
	"net/http"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-cache/vectorDBProvider"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/wrapper"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/tidwall/gjson"
	"github.com/tidwall/resp"
)

type CacheConfig struct {
	// @Title zh-CN redis 服务名称
	// @Description zh-CN 带服务类型的完整 FQDN 名称，例如 my-redis.dns、redis.my-ns.svc.cluster.local
	RedisServiceName string `required:"true" yaml:"serviceName" json:"serviceName"`
	// @Title zh-CN redis 服务端口
	// @Description zh-CN 默认值为6379
	RedisServicePort int `required:"false" yaml:"servicePort" json:"servicePort"`
	// @Title zh-CN 用户名
	// @Description zh-CN 登陆 redis 的用户名，非必填
	RedisUsername string `required:"false" yaml:"username" json:"username"`
	// @Title zh-CN 密码
	// @Description zh-CN 登陆 redis 的密码，非必填，可以只填密码
	RedisPassword string `required:"false" yaml:"password" json:"password"`
	// @Title zh-CN 请求超时
	// @Description zh-CN 请求 redis 的超时时间，单位为毫秒。默认值是1000，即1秒
	RedisTimeout int `required:"false" yaml:"timeout" json:"timeout"`

	redisClient wrapper.RedisClient `yaml:"-" json:"-"`

	activeEmbeddingProvider vectorDBProvider.ProviderConfig `yaml:"-" json:"-"`

	activeVectorDBProvider vectorDBProvider.ProviderConfig `yaml:"-" json:"-"`
}

func (c *CacheConfig) FromJson(json gjson.Result) {
	c.RedisServiceName = json.Get("RedisServiceName").String()
	c.RedisServicePort = int(json.Get("RedisServicePort").Int())
	c.RedisUsername = json.Get("RedisUsername").String()
	c.RedisPassword = json.Get("RedisPassword").String()
	c.RedisTimeout = int(json.Get("RedisTimeout").Int())
}

// ===================== 以下是主要逻辑 =====================
// 主handler函数，根据key从redis中获取value ，如果不命中，则首先调用文本向量化接口向量化query，然后调用向量搜索接口搜索最相似的出现过的key，最后再次调用redis获取结果
// 可以把所有handler单独提取为文件，这里为了方便读者复制就和主逻辑放在一个文件中了
//
// 1. query 进来和 redis 中存的 key 匹配 (redisSearchHandler) ，若完全一致则直接返回 (handleCacheHit)
// 2. 否则请求 text_embdding 接口将 query 转换为 query_embedding (fetchAndProcessEmbeddings)
// 3. 用 query_embedding 和向量数据库中的向量做 ANN search，返回最接近的 key ，并用阈值过滤 (performQueryAndRespond)
// 4. 若返回结果为空或大于阈值，舍去，本轮 cache 未命中, 最后将 query_embedding 存入向量数据库 (uploadQueryEmbedding)
// 5. 若小于阈值，则再次调用 redis对 most similar key 做匹配。 (redisSearchHandler)
// 7. 在 response 阶段请求 redis 新增key/LLM返回结果

func redisSearchHandler(key string, ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log, stream bool, ifUseEmbedding bool) error {
	err := config.redisClient.Get(config.CacheKeyPrefix+key, func(response resp.Value) {
		if err := response.Error(); err == nil && !response.IsNull() {
			log.Warnf("cache hit, key:%s", key)
			handleCacheHit(key, response, stream, ctx, config, log)
		} else {
			log.Warnf("cache miss, key:%s", key)
			if ifUseEmbedding {
				handleCacheMiss(key, err, response, ctx, config, log, key, stream)
			} else {
				proxywasm.ResumeHttpRequest()
				return
			}
		}
	})
	return err
}

// 简单处理缓存命中的情况, 从redis中获取到value后，直接返回
func handleCacheHit(key string, response resp.Value, stream bool, ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log) {
	log.Warnf("cache hit, key:%s", key)
	ctx.SetContext(CacheKeyContextKey, nil)
	if !stream {
		proxywasm.SendHttpResponse(200, [][2]string{{"content-type", "application/json; charset=utf-8"}}, []byte(fmt.Sprintf(config.ReturnResponseTemplate, response.String())), -1)
	} else {
		proxywasm.SendHttpResponse(200, [][2]string{{"content-type", "text/event-stream; charset=utf-8"}}, []byte(fmt.Sprintf(config.ReturnStreamResponseTemplate, response.String())), -1)
	}
}

// 处理缓存未命中的情况，调用fetchAndProcessEmbeddings函数向量化query
func handleCacheMiss(key string, err error, response resp.Value, ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log, queryString string, stream bool) {
	if err != nil {
		log.Warnf("redis get key:%s failed, err:%v", key, err)
	}
	if response.IsNull() {
		log.Warnf("cache miss, key:%s", key)
	}
	fetchAndProcessEmbeddings(key, ctx, config, log, queryString, stream)
}

// 调用文本向量化接口向量化query, 向量化成功后调用processFetchedEmbeddings函数处理向量化结果
func fetchAndProcessEmbeddings(key string, ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log, queryString string, stream bool) {
	Emb_url, Emb_requestBody, Emb_headers := ConstructTextEmbeddingParameters(&config, log, []string{queryString})
	config.DashVectorInfo.DashScopeClient.Post(
		Emb_url,
		Emb_headers,
		Emb_requestBody,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			// log.Infof("statusCode:%d, responseBody:%s", statusCode, string(responseBody))
			log.Infof("Successfully fetched embeddings for key: %s", key)
			if statusCode != 200 {
				log.Errorf("Failed to fetch embeddings, statusCode: %d, responseBody: %s", statusCode, string(responseBody))
				ctx.SetContext(QueryEmbeddingKey, nil)
				proxywasm.ResumeHttpRequest()
			} else {
				processFetchedEmbeddings(key, responseBody, ctx, config, log, stream)
			}
		},
		10000)
}

// 先将向量化的结果存入上下文ctx变量，其次发起向量搜索请求
func processFetchedEmbeddings(key string, responseBody []byte, ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log, stream bool) {
	text_embedding_raw, _ := ParseTextEmbedding(responseBody)
	text_embedding := text_embedding_raw.Output.Embeddings[0].Embedding
	// ctx.SetContext(CacheKeyContextKey, text_embedding)
	ctx.SetContext(QueryEmbeddingKey, text_embedding)
	ctx.SetContext(CacheKeyContextKey, key)
	performQueryAndRespond(key, text_embedding, ctx, config, log, stream)
}

// 调用向量搜索接口搜索最相似的key，搜索成功后调用redisSearchHandler函数获取最相似的key的结果
func performQueryAndRespond(key string, text_embedding []float64, ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log, stream bool) {
	vector_url, vector_request, vector_headers, err := ConstructEmbeddingQueryParameters(config, text_embedding)
	if err != nil {
		log.Errorf("Failed to perform query, err: %v", err)
		proxywasm.ResumeHttpRequest()
		return
	}
	config.DashVectorInfo.DashVectorClient.Post(
		vector_url,
		vector_headers,
		vector_request,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			log.Infof("statusCode:%d, responseBody:%s", statusCode, string(responseBody))
			query_resp, err_query := ParseQueryResponse(responseBody)
			if err_query != nil {
				log.Errorf("Failed to parse response: %v", err)
				proxywasm.ResumeHttpRequest()
				return
			}
			if len(query_resp.Output) < 1 {
				log.Warnf("query response is empty")
				uploadQueryEmbedding(ctx, config, log, key, text_embedding)
				return
			}
			most_similar_key := query_resp.Output[0].Fields["query"].(string)
			log.Infof("most similar key:%s", most_similar_key)
			most_similar_score := query_resp.Output[0].Score
			if most_similar_score < 0.1 {
				ctx.SetContext(CacheKeyContextKey, nil)
				redisSearchHandler(most_similar_key, ctx, config, log, stream, false)
			} else {
				log.Infof("the most similar key's score is too high, key:%s, score:%f", most_similar_key, most_similar_score)
				uploadQueryEmbedding(ctx, config, log, key, text_embedding)
				proxywasm.ResumeHttpRequest()
				return
			}
		},
		100000)
}

// 未命中cache，则将新的query embedding和对应的key存入向量数据库
func uploadQueryEmbedding(ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log, key string, text_embedding []float64) error {
	vector_url, vector_body, err := ConsturctEmbeddingInsertParameters(&config, log, text_embedding, key)
	if err != nil {
		log.Errorf("Failed to construct embedding insert parameters: %v", err)
		proxywasm.ResumeHttpRequest()
		return nil
	}
	err = config.DashVectorInfo.DashVectorClient.Post(
		vector_url,
		[][2]string{
			{"Content-Type", "application/json"},
			{"dashvector-auth-token", config.DashVectorInfo.DashVectorKey},
		},
		vector_body,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			if statusCode != 200 {
				log.Errorf("Failed to upload query embedding: %s", responseBody)
			} else {
				log.Infof("Successfully uploaded query embedding for key: %s", key)
			}
			proxywasm.ResumeHttpRequest()
		},
		10000,
	)
	if err != nil {
		log.Errorf("Failed to upload query embedding: %v", err)
		proxywasm.ResumeHttpRequest()
		return nil
	}
	return nil
}

// ===================== 以上是主要逻辑 =====================
