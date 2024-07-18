// 这个文件中主要将OnHttpRequestHeaders、OnHttpRequestBody、OnHttpResponseHeaders、OnHttpResponseBody这四个函数实现
// 其中的缓存思路调用cache.go中的逻辑，然后cache.go中的逻辑会调用textEmbeddingProvider和vectorStoreProvider中的逻辑（实例）
package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-cache/config"
	provider "github.com/alibaba/higress/plugins/wasm-go/extensions/ai-cache/vectordatabaseProvider"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/wrapper"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
	"github.com/tidwall/resp"
)

const (
	CacheKeyContextKey       = "cacheKey"
	CacheContentContextKey   = "cacheContent"
	PartialMessageContextKey = "partialMessage"
	ToolCallsContextKey      = "toolCalls"
	StreamContextKey         = "stream"
	CacheKeyPrefix           = "higressAiCache"
	DefaultCacheKeyPrefix    = "higressAiCache"
	QueryEmbeddingKey        = "queryEmbedding"
)

func main() {
	wrapper.SetCtx(
		"ai-cache",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBodyBy(onHttpResponseBody),
	)
}

func parseConfig(json gjson.Result, config.PluginConfig provider.config.PluginConfig, log wrapper.Log) error {
	config.PluginConfig.EmbeddingProviderConfig.FromJson(json.Get("embeddingProvider"))
	config.PluginConfig.VectorBaseProviderConfig.FromJson(json.Get("vectorBaseProvider"))
	if err := config.PluginConfig.Validate(); err != nil {
		return err
	}
	return nil
}

func TrimQuote(source string) string {
	return strings.Trim(source, `"`)
}

func onHttpRequestBody(ctx wrapper.HttpContext, config config.PluginConfig, body []byte, log wrapper.Log) types.Action {
	activeEmbeddingProvider := config.EmbeddingProviderConfig.GetProvider()
	activeVectorBaseProvider := config.VectorBaseProviderConfig.GetProvider()

	bodyJson := gjson.ParseBytes(body)
	// TODO: It may be necessary to support stream mode determination for different LLM providers.
	stream := false
	if bodyJson.Get("stream").Bool() {
		stream = true
		ctx.SetContext(StreamContextKey, struct{}{})
	} else if ctx.GetContext(StreamContextKey) != nil {
		stream = true
	}
	// key := TrimQuote(bodyJson.Get(config.CacheKeyFrom.RequestBody).Raw)
	key := bodyJson.Get(config.CacheKeyFrom.RequestBody).String()
	if key == "" {
		log.Debug("parse key from request body failed")
		return types.ActionContinue
	}

	queryString := config.CacheKeyPrefix + key

	err := redisSearchHandler(queryString, ctx, config, log, stream, true)

	if err != nil {
		log.Error("redis access failed")
		return types.ActionContinue
	}
	return types.ActionPause
}

func processSSEMessage(ctx wrapper.HttpContext, config config.PluginConfig, sseMessage string, log wrapper.Log) string {
	subMessages := strings.Split(sseMessage, "\n")
	var message string
	for _, msg := range subMessages {
		if strings.HasPrefix(msg, "data:") {
			message = msg
			break
		}
	}
	if len(message) < 6 {
		log.Warnf("invalid message:%s", message)
		return ""
	}
	// skip the prefix "data:"
	bodyJson := message[5:]
	if gjson.Get(bodyJson, config.CacheStreamValueFrom.ResponseBody).Exists() {
		tempContentI := ctx.GetContext(CacheContentContextKey)
		if tempContentI == nil {
			content := TrimQuote(gjson.Get(bodyJson, config.CacheStreamValueFrom.ResponseBody).Raw)
			ctx.SetContext(CacheContentContextKey, content)
			return content
		}
		append := TrimQuote(gjson.Get(bodyJson, config.CacheStreamValueFrom.ResponseBody).Raw)
		content := tempContentI.(string) + append
		ctx.SetContext(CacheContentContextKey, content)
		return content
	} else if gjson.Get(bodyJson, "choices.0.delta.content.tool_calls").Exists() {
		// TODO: compatible with other providers
		ctx.SetContext(ToolCallsContextKey, struct{}{})
		return ""
	}
	log.Warnf("unknown message:%s", bodyJson)
	return ""
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config config.config.PluginConfig, log wrapper.Log) types.Action {
	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
	if strings.Contains(contentType, "text/event-stream") {
		ctx.SetContext(StreamContextKey, struct{}{})
	}
	return types.ActionContinue
}

func onHttpResponseBody(ctx wrapper.HttpContext, config config.PluginConfig, chunk []byte, isLastChunk bool, log wrapper.Log) []byte {
	// log.Infof("I am here")
	if ctx.GetContext(ToolCallsContextKey) != nil {
		// we should not cache tool call result
		return chunk
	}
	keyI := ctx.GetContext(CacheKeyContextKey)
	// log.Infof("I am here 2: %v", keyI)
	if keyI == nil {
		return chunk
	}
	if !isLastChunk {
		stream := ctx.GetContext(StreamContextKey)
		if stream == nil {
			tempContentI := ctx.GetContext(CacheContentContextKey)
			if tempContentI == nil {
				ctx.SetContext(CacheContentContextKey, chunk)
				return chunk
			}
			tempContent := tempContentI.([]byte)
			tempContent = append(tempContent, chunk...)
			ctx.SetContext(CacheContentContextKey, tempContent)
		} else {
			var partialMessage []byte
			partialMessageI := ctx.GetContext(PartialMessageContextKey)
			if partialMessageI != nil {
				partialMessage = append(partialMessageI.([]byte), chunk...)
			} else {
				partialMessage = chunk
			}
			messages := strings.Split(string(partialMessage), "\n\n")
			for i, msg := range messages {
				if i < len(messages)-1 {
					// process complete message
					processSSEMessage(ctx, config, msg, log)
				}
			}
			if !strings.HasSuffix(string(partialMessage), "\n\n") {
				ctx.SetContext(PartialMessageContextKey, []byte(messages[len(messages)-1]))
			} else {
				ctx.SetContext(PartialMessageContextKey, nil)
			}
		}
		return chunk
	}
	// last chunk
	key := keyI.(string)
	stream := ctx.GetContext(StreamContextKey)
	var value string
	if stream == nil {
		var body []byte
		tempContentI := ctx.GetContext(CacheContentContextKey)
		if tempContentI != nil {
			body = append(tempContentI.([]byte), chunk...)
		} else {
			body = chunk
		}
		bodyJson := gjson.ParseBytes(body)

		value = TrimQuote(bodyJson.Get(config.CacheValueFrom.ResponseBody).Raw)
		if value == "" {
			log.Warnf("parse value from response body failded, body:%s", body)
			return chunk
		}
	} else {
		if len(chunk) > 0 {
			var lastMessage []byte
			partialMessageI := ctx.GetContext(PartialMessageContextKey)
			if partialMessageI != nil {
				lastMessage = append(partialMessageI.([]byte), chunk...)
			} else {
				lastMessage = chunk
			}
			if !strings.HasSuffix(string(lastMessage), "\n\n") {
				log.Warnf("invalid lastMessage:%s", lastMessage)
				return chunk
			}
			// remove the last \n\n
			lastMessage = lastMessage[:len(lastMessage)-2]
			value = processSSEMessage(ctx, config, string(lastMessage), log)
		} else {
			tempContentI := ctx.GetContext(CacheContentContextKey)
			if tempContentI == nil {
				return chunk
			}
			value = tempContentI.(string)
		}
	}
	log.Infof("I am processing cache to redis, key:%s, value:%s", key, value)
	config.redisClient.Set(config.CacheKeyPrefix+key, value, nil)
	if config.CacheTTL != 0 {
		config.redisClient.Expire(config.CacheKeyPrefix+key, config.CacheTTL, nil)
	}
	return chunk
}
