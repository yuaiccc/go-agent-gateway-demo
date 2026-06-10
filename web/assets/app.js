const tenantSelect = document.querySelector("#tenant");
const userIdInput = document.querySelector("#userId");
const sessionIdInput = document.querySelector("#sessionId");
const modelName = document.querySelector("#modelName");
const modelProvider = document.querySelector("#modelProvider");
const modelToggleBtn = document.querySelector("#modelToggleBtn");
const toolList = document.querySelector("#toolList");
const timeline = document.querySelector("#timeline");
const form = document.querySelector("#chatForm");
const messageInput = document.querySelector("#message");
const sendBtn = document.querySelector("#sendBtn");
const clearBtn = document.querySelector("#clearBtn");

let tenants = [];

const eventNames = {
  ready: "准备就绪",
  user_message: "用户消息",
  run_start: "运行开始",
  tool_call_start: "工具调用开始",
  tool_call_result: "工具调用结果",
  message_delta: "回答增量",
  done: "运行完成",
  error: "错误",
  model_updated: "模型已切换",
  session_reset: "会话已重置",
};

const tenantNames = {
  "tenant-jp": "日语学习租户",
  "tenant-code": "代码智能体租户",
};

const toolNames = {
  search_grammar: "语法检索",
  search_memory: "记忆检索",
};

function newSessionId() {
  return `sess-${Date.now()}`;
}

function appendEvent(type, payload) {
  const node = document.createElement("article");
  node.className = `event ${type === "error" ? "error" : ""}`;
  node.innerHTML = `
    <div class="event-head">
      <span class="event-type">${eventNames[type] ?? type}</span>
      <span>${new Date().toLocaleTimeString()}</span>
    </div>
    <pre>${escapeHtml(JSON.stringify(payload, null, 2))}</pre>
  `;
  timeline.appendChild(node);
  timeline.scrollTop = timeline.scrollHeight;
}

function appendAnswerNode() {
  const node = document.createElement("div");
  node.className = "answer";
  timeline.appendChild(node);
  return node;
}

function escapeHtml(value) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

async function loadTenants() {
  const res = await fetch("/api/tenants");
  const data = await res.json();
  tenants = data.tenants ?? [];
  tenantSelect.innerHTML = tenants
    .map(
      (tenant) =>
        `<option value="${tenant.id}">${tenantNames[tenant.id] ?? tenant.name}</option>`,
    )
    .join("");
  if (tenants.some((tenant) => tenant.id === "tenant-jp")) {
    tenantSelect.value = "tenant-jp";
  }
  sessionIdInput.value = newSessionId();
  updateModelCard();
}

async function loadTools() {
  const res = await fetch("/mcp/tools/list");
  const data = await res.json();
  toolList.innerHTML = (data.tools ?? [])
    .map(
      (tool) => `
        <li>
          <strong>${tool.name}</strong>
          ${toolNames[tool.name] ?? tool.description}
        </li>
      `,
    )
    .join("");
}

function selectedTenant() {
  return tenants.find((tenant) => tenant.id === tenantSelect.value);
}

function updateModelCard() {
  const tenant = selectedTenant();
  if (!tenant) return;
  modelName.textContent = tenant.model.model;
  modelProvider.textContent = `供应商：${tenant.model.provider}，温度：${tenant.model.temperature}`;
  modelToggleBtn.textContent =
    tenant.model.provider === "deepseek" ? "切回模拟模型" : "切到 DeepSeek";
}

function switchTenant() {
  sessionIdInput.value = newSessionId();
  updateModelCard();
  appendEvent("session_reset", {
    session_id: sessionIdInput.value,
    reason: "租户已切换，已自动创建新的会话，避免和旧租户串号。",
  });
}

function parseSSEChunk(buffer, onEvent) {
  const parts = buffer.split("\n\n");
  const rest = parts.pop() ?? "";

  for (const part of parts) {
    const lines = part.split("\n");
    const eventLine = lines.find((line) => line.startsWith("event:"));
    const dataLine = lines.find((line) => line.startsWith("data:"));
    if (!eventLine || !dataLine) continue;

    const type = eventLine.slice("event:".length).trim();
    const rawData = dataLine.slice("data:".length).trim();
    try {
      onEvent(type, JSON.parse(rawData));
    } catch {
      onEvent(type, { raw: rawData });
    }
  }

  return rest;
}

async function streamAgent(message, retryOnForbidden = true) {
  sendBtn.disabled = true;
  const answerNode = appendAnswerNode();
  let buffer = "";

  const res = await fetch("/api/agent/stream", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      tenant_id: tenantSelect.value,
      user_id: userIdInput.value,
      session_id: sessionIdInput.value,
      message,
    }),
  });

  if (!res.ok || !res.body) {
    const text = await res.text();
    let body = text;
    try {
      body = JSON.parse(text);
    } catch {}
    appendEvent("error", {
      status: res.status,
      body,
      hint:
        res.status === 403
          ? "当前会话 ID 已经属于其他租户或用户。已自动换一个新会话。"
          : undefined,
    });
    if (res.status === 403 && retryOnForbidden) {
      sessionIdInput.value = newSessionId();
      answerNode.remove();
      appendEvent("session_reset", {
        session_id: sessionIdInput.value,
        reason: "会话归属冲突，已自动创建新会话并重试。",
      });
      await streamAgent(message, false);
      sendBtn.disabled = false;
      return;
    }
    if (res.status === 403) {
      sessionIdInput.value = newSessionId();
    }
    sendBtn.disabled = false;
    return;
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    buffer = parseSSEChunk(buffer, (type, payload) => {
      if (type === "message_delta") {
        answerNode.textContent += payload.delta ?? "";
        timeline.scrollTop = timeline.scrollHeight;
        return;
      }
      appendEvent(type, payload);
    });
  }

  sendBtn.disabled = false;
}

tenantSelect.addEventListener("change", switchTenant);
modelToggleBtn.addEventListener("click", async () => {
  const tenant = selectedTenant();
  const nextModel =
    tenant?.model.provider === "deepseek"
      ? {
          provider: "mock",
          model: "mock-japanese-tutor",
          temperature: 0.2,
        }
      : {
          provider: "deepseek",
          model: "deepseek-chat",
          temperature: 0.3,
        };
  const res = await fetch(`/api/tenants/${tenantSelect.value}/model`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(nextModel),
  });
  const updated = await res.json();
  tenants = tenants.map((tenant) => (tenant.id === updated.id ? updated : tenant));
  updateModelCard();
  appendEvent("model_updated", updated.model ?? updated);
});
clearBtn.addEventListener("click", () => {
  timeline.innerHTML = "";
});

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  const message = messageInput.value.trim();
  if (!message) return;
  appendEvent("user_message", { message });
  await streamAgent(message);
});

await loadTenants();
await loadTools();
appendEvent("ready", {
  hint: "发送一条消息，观察智能体网关如何推送工具调用和流式答案。",
});
