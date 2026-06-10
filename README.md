# Go Agent Gateway Demo

一个用于学习 Agent 后端平台的最小 Go/Gin demo。它不是完整生产系统，而是把面试里常见的概念拆成可运行代码：

- 多租户：`tenant_id` 决定模型配置、知识库和可用工具。
- Session/User 绑定：同一个 `session_id` 只能属于一个 `tenant_id + user_id`。
- Tool Registry：工具注册、发现、调用和权限检查。
- SSE 流式事件：推送 `run_start`、`tool_call_start`、`tool_call_result`、`message_delta`、`done`。
- MCP-style API：提供简化版 `/mcp/tools/list` 和 `/mcp/tools/call`。

## Run

```bash
cd /Users/xujunshan/Code/go-agent-gateway-demo
go mod tidy
go run ./cmd/server
```

默认监听：

```text
http://localhost:8088
```

## API

### Health

```bash
curl http://localhost:8088/healthz
```

### List Tenants

```bash
curl http://localhost:8088/api/tenants
```

### Hot Update Tenant Model

这个接口模拟“多租户模型热更新”。只会影响目标租户，不影响其他租户。

```bash
curl -X PATCH http://localhost:8088/api/tenants/tenant-jp/model \
  -H 'Content-Type: application/json' \
  -d '{"provider":"mock","model":"mock-japanese-tutor-v2","temperature":0.3}'
```

### Stream Agent

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

### MCP-style Tools

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

## Architecture

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
internal/tool/registry.go   工具注册、发现、调用
internal/tenant/store.go    租户模型配置和热更新
internal/session/store.go   session 与 user/tenant 绑定
```

## Interview Mapping

这个 demo 对应面试题：

- 多模型支持架构：看 `tenant.ModelConfig`。
- 多租户模型热更新：看 `PATCH /api/tenants/:tenantID/model`。
- Agent 状态：看 SSE event type。
- Session/User 绑定：看 `session.Store.ValidateOwner`。
- 工具调用流程：看 `agent.Service.Run` 和 `tool.Registry.Call`。
- SSE 数据格式：看 `gateway.streamAgent`。
- MCP 交互流程：看 `/mcp/tools/list` 和 `/mcp/tools/call`。

## What This Demo Omits

为了保持教学清晰，暂时没有做：

- 真实 LLM API 调用。
- 真实 MCP JSON-RPC 协议握手。
- 数据库持久化。
- JWT/Auth/RBAC。
- Docker sandbox。
- 向量库和 embedding。

这些可以作为下一阶段扩展。
