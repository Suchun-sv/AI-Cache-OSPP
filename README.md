# AI-Cache-OSPP
 Code for AI Cache Plugin

# 0716 思路：利用Redis作为缓存，把textEmbeddingProvider和vectorStoreProvider做为可替换部分
1. 在main函数中实现OnHttpRequestHeaders、OnHttpRequestBody、OnHttpResponseHeaders、OnHttpResponseBody这四个函数，其中只有OnHttpRequestBody需要根据不同的配置进行不同的处理
2. 具体来说，在OnHttpRequestBody函数里拿到请求的body（query的字符串）后，执行cache.go中的缓存逻辑，首先逐字匹配query和缓存中的key，如果匹配成功则直接返回缓存中的value，否则首先调用textEmbeddingProvider的GetTextEmbedding方法得到query的embedding，然后调用vectorStoreProvider的GetVector方法得到query的vector。
3. 判断分数后，如果相似度分数小于阈值，则直接返回缓存中的value，否则将query的embedding和vector存入缓存中并resume请求。
4. 最后在OnHttpResponseBody函数中，将大模型返回的结果存入redis缓存中。

代码框架：
```bash
├── cache.go //缓存逻辑，在这里调用textEmbeddingProvider和vectorStoreProvider
├── config
│   └── config.go
├── go.mod
├── go.sum
├── main.go // 主要四个函数的逻辑，缓存部分主要实现还在cache.go中
├── option.yaml
├── README.md
├── textEmbeddingProvider // 类似AI-proxy中的相应配置，主要在provider.go中暴露接口，其他具体实现可以单独写一个go文件
│   ├── dashscope.go
│   └── provider.go
└── vectorStoreProvider // 类似AI-proxy中的相应配置，主要在provider.go中暴露接口，其他具体实现可以单独写一个go文件
    ├── dashvector.go
    └── provider.go
```