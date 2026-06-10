# Go 智能体网关演示

一个用于学习智能体后端平台的最小 Go/Gin 演示项目。它不是完整生产系统，而是把面试里常见的概念拆成可运行代码：

- 多租户：`tenant_id` 决定模型配置、知识库和可用工具。
- 会话/用户绑定：同一个 `session_id` 只能属于一个 `tenant_id + user_id`，并持久化到 SQLite。
- Tool Registry：工具注册、发现、调用和权限检查。
- SSE 流式事件：推送 `run_start`、`tool_call_start`、`tool_call_result`、`message_delta`、`done`。
- MCP JSON-RPC：`/mcp` 支持 `initialize`、`tools/list`、`tools/call`，同时保留简化版 `/mcp/tools/list` 和 `/mcp/tools/call`。
- 内置前端：访问 `/` 直接观察租户、工具调用和 SSE 流式输出。
- DeepSeek/OpenAI 兼容模型适配器：通过环境变量接真实模型，并把上游 streaming delta 原样转成前端 SSE。

## 运行

```bash
cd /Users/xujunshan/Code/go-agent-gateway-demo
go mod tidy
go run ./cmd/server
```

默认监听：

```text
http://localhost:8088
```

运行后会自动创建 SQLite：

```text
data/gateway.sqlite
```

默认的 `tenant-jp` 会走 DeepSeek。启动前需要设置：

```bash
export DEEPSEEK_API_KEY=你的_key
go run ./cmd/server
```

然后把租户模型切到 DeepSeek：
如果你想手动切模型，也可以调用热更新接口。例如切回 mock：

```bash
curl -X PATCH http://localhost:8088/api/tenants/tenant-jp/model \
  -H 'Content-Type: application/json' \
  -d '{"provider":"mock","model":"mock-japanese-tutor","temperature":0.2}'
```

打开浏览器访问：

```text
http://localhost:8088
```

## API

### 健康检查

```bash
curl http://localhost:8088/healthz
```

### 查看租户

```bash
curl http://localhost:8088/api/tenants
```

### 热更新租户模型

这个接口模拟“多租户模型热更新”。只会影响目标租户，不影响其他租户。

```bash
curl -X PATCH http://localhost:8088/api/tenants/tenant-jp/model \
  -H 'Content-Type: application/json' \
  -d '{"provider":"mock","model":"mock-japanese-tutor-v2","temperature":0.3}'
```

### 流式调用智能体

```bash
curl -N -X POST http://localhost:8088/api/agent/stream \
  -H 'Content-Type: application/json' \
  -d '{
    "tenant_id": "tenant-jp",
    "user_id": "user-001",
    "session_id": "sess-demo",
    "message": "食べる的て形是什么？顺便看看我的记忆薄弱点"
  }'
```

你会看到 SSE 事件：

```text
event: run_start
data: {...}

event: tool_call_start
data: {"tool_name":"search_grammar",...}

event: tool_call_result
data: {...}

event: message_delta
data: {"delta":"这",...}

event: done
data: {...}
```

### MCP 风格工具

JSON-RPC 入口：

```bash
curl -X POST http://localhost:8088/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

调用工具：

```bash
curl -X POST http://localhost:8088/mcp \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/call",
    "params": {
      "name": "search_grammar",
      "arguments": {
        "query": "召し上がる 尊敬语",
        "top_k": 2
      }
    }
  }'
```

兼容旧的教学端点：

列出工具：

```bash
curl http://localhost:8088/mcp/tools/list
```

调用工具：

```bash
curl -X POST http://localhost:8088/mcp/tools/call \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "call-1",
    "name": "search_grammar",
    "arguments": {
      "query": "召し上がる 尊敬语"
    }
  }'
```

记忆检索会查询 SQLite，需要租户和用户：

```bash
curl -X POST http://localhost:8088/mcp \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "search_memory",
      "arguments": {
        "tenant_id": "tenant-jp",
        "user_id": "user-001",
        "query": "记忆薄弱点"
      }
    }
  }'
```

## 架构

```text
HTTP/SSE Client
  -> Gin Gateway
  -> Tenant Store
  -> Session Store
  -> Agent Service
  -> Tool Registry
  -> Tool Handler
  -> SSE Events
```

目录：

```text
cmd/server/main.go          启动入口
internal/gateway/server.go  Gin API、SSE、MCP-style endpoint
internal/agent/agent.go     简化 tool-call loop
internal/model/client.go    mock / DeepSeek provider 适配
internal/tool/registry.go   工具注册、发现、调用
internal/tool/grammar.go    本地 markdown chunk 加载和检索
internal/tool/memory.go     SQLite memory 检索
internal/store/db.go        SQLite migration 和 seed
internal/tenant/store.go    租户模型配置和热更新
internal/session/store.go   session 与 user/tenant 绑定和持久化
data/grammar/*.md           本地语法文档 chunk 来源
web/index.html              无构建步骤的演示前端
web/assets/app.js           fetch + ReadableStream 解析 SSE
```

## 面试题对应关系

这个 demo 对应面试题：

- 多模型支持架构：看 `tenant.ModelConfig`。
- 多租户模型热更新：看 `PATCH /api/tenants/:tenantID/model`。
- 模型 provider 适配：看 `internal/model/client.go`。
- 智能体状态：看 SSE event type。
- 会话/用户绑定：看 `session.Store.ValidateOwner`。
- 工具调用流程：看 `agent.Service.Run` 和 `tool.Registry.Call`。
- SSE 数据格式：看 `gateway.streamAgent`。
- MCP 交互流程：看 `POST /mcp` 的 JSON-RPC `initialize`、`tools/list`、`tools/call`。
- RAG/检索：看 `search_grammar` 如何从 `data/grammar/*.md` chunk 检索。
- 记忆：看 `search_memory` 如何按 `tenant_id + user_id` 查询 SQLite。

## 暂未实现

为了保持教学清晰，暂时没有做：

- JWT/Auth/RBAC。
- Docker sandbox。
- 向量库、embedding 和 reranker。
- 完整 MCP SDK transport/session 生命周期。
- provider 密钥托管、限流、重试和熔断。

其中 `search_grammar` 是本地文档 chunk 的关键词检索，不是向量检索；`search_memory` 和 session 已经落 SQLite。
