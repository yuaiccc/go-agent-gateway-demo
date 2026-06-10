package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yuaiccc/go-agent-gateway-demo/internal/agent"
	"github.com/yuaiccc/go-agent-gateway-demo/internal/session"
	"github.com/yuaiccc/go-agent-gateway-demo/internal/store"
	"github.com/yuaiccc/go-agent-gateway-demo/internal/tenant"
	"github.com/yuaiccc/go-agent-gateway-demo/internal/tool"
)

type Server struct {
	Tenants  *tenant.Store
	Sessions *session.Store
	Tools    *tool.Registry
	Agent    *agent.Service
	DB       *store.DB
}

func NewServer() *Server {
	db, err := store.Open("")
	if err != nil {
		panic(err)
	}
	grammar, err := tool.LoadGrammarIndex("")
	if err != nil {
		panic(err)
	}
	tools := tool.NewRegistryWithStores(grammar, tool.NewMemoryStore(db.SQL))
	return &Server{
		Tenants:  tenant.NewStore(),
		Sessions: session.NewStore(db.SQL),
		Tools:    tools,
		Agent:    agent.NewService(tools),
		DB:       db,
	}
}

func (s *Server) Router() *gin.Engine {
	r := gin.Default()

	r.StaticFile("/", "./web/index.html")
	r.Static("/assets", "./web/assets")

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.GET("/api/tenants", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"tenants": s.Tenants.List()})
	})

	r.PATCH("/api/tenants/:tenantID/model", s.updateTenantModel)
	r.POST("/api/agent/stream", s.streamAgent)

	mcp := r.Group("/mcp")
	mcp.POST("", s.handleMCPJSONRPC)
	mcp.GET("/tools/list", s.listMCPTools)
	mcp.POST("/tools/call", s.callMCPTool)

	return r
}

func (s *Server) updateTenantModel(c *gin.Context) {
	var req tenant.ModelConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := s.Tenants.UpdateModel(c.Param("tenantID"), req)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Server) streamAgent(c *gin.Context) {
	var req agent.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.TenantID = strings.TrimSpace(req.TenantID)
	req.UserID = strings.TrimSpace(req.UserID)
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Message = strings.TrimSpace(req.Message)
	if req.TenantID == "" || req.UserID == "" || req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id, user_id and message are required"})
		return
	}
	if req.SessionID == "" {
		req.SessionID = fmt.Sprintf("sess-%d", time.Now().UnixMilli())
	}

	cfg, ok := s.Tenants.Get(req.TenantID)
	if !ok || !cfg.Active {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found or inactive"})
		return
	}
	if err := s.Sessions.ValidateOwner(c.Request.Context(), req.SessionID, req.TenantID, req.UserID); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	if _, err := s.Sessions.GetOrCreate(c.Request.Context(), req.TenantID, req.UserID, req.SessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if _, err := s.Sessions.Append(c.Request.Context(), req.SessionID, "user", req.Message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	writer := c.Writer
	flusher, ok := writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	var finalAnswer string
	emit := func(ev agent.Event) error {
		if ev.Type == "done" {
			if data, ok := ev.Data.(map[string]any); ok {
				if answer, ok := data["answer"].(string); ok {
					finalAnswer = answer
				}
			}
		}
		payload, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", ev.Type, payload)
		flusher.Flush()
		return err
	}

	if err := s.Agent.Run(c.Request.Context(), cfg, req, emit); err != nil {
		_ = emit(agent.Event{
			Type:      "error",
			TenantID:  req.TenantID,
			UserID:    req.UserID,
			SessionID: req.SessionID,
			Status:    "failed",
			Error:     err.Error(),
			Timestamp: time.Now(),
		})
		return
	}
	if finalAnswer != "" {
		if _, err := s.Sessions.Append(c.Request.Context(), req.SessionID, "assistant", finalAnswer); err != nil {
			_ = emit(agent.Event{
				Type:      "error",
				TenantID:  req.TenantID,
				UserID:    req.UserID,
				SessionID: req.SessionID,
				Status:    "failed",
				Error:     err.Error(),
				Timestamp: time.Now(),
			})
		}
	}
}

func (s *Server) listMCPTools(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"tools": s.Tools.List(),
	})
}

func (s *Server) callMCPTool(c *gin.Context) {
	var req tool.Call
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ID == "" {
		req.ID = fmt.Sprintf("call-%d", time.Now().UnixMilli())
	}
	result, err := s.Tools.Call(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id,omitempty"`
	Result  any           `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) handleMCPJSONRPC(c *gin.Context) {
	var req jsonRPCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, jsonRPCResponse{
			JSONRPC: "2.0",
			Error:   &jsonRPCError{Code: -32700, Message: err.Error()},
		})
		return
	}
	if req.JSONRPC == "" {
		req.JSONRPC = "2.0"
	}

	result, rpcErr := s.dispatchMCP(c, req)
	status := http.StatusOK
	resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
	if rpcErr != nil {
		resp.Result = nil
		resp.Error = rpcErr
		if rpcErr.Code == -32602 {
			status = http.StatusBadRequest
		}
	}
	c.JSON(status, resp)
}

func (s *Server) dispatchMCP(c *gin.Context, req jsonRPCRequest) (any, *jsonRPCError) {
	switch req.Method {
	case "initialize":
		return gin.H{
			"protocolVersion": "2024-11-05",
			"serverInfo": gin.H{
				"name":    "go-agent-gateway-demo",
				"version": "0.2.0",
			},
			"capabilities": gin.H{
				"tools": gin.H{"listChanged": true},
			},
		}, nil
	case "tools/list":
		return gin.H{"tools": s.Tools.List()}, nil
	case "tools/call":
		var params struct {
			ID        string         `json:"id"`
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &jsonRPCError{Code: -32602, Message: err.Error()}
		}
		if params.ID == "" {
			params.ID = fmt.Sprintf("call-%d", time.Now().UnixMilli())
		}
		result, err := s.Tools.Call(c.Request.Context(), tool.Call{
			ID:        params.ID,
			Name:      params.Name,
			Arguments: params.Arguments,
		})
		if err != nil {
			return nil, &jsonRPCError{Code: -32602, Message: err.Error()}
		}
		return gin.H{
			"content": []gin.H{
				{
					"type": "json",
					"json": result.Content,
				},
			},
			"tool_result": result,
		}, nil
	default:
		return nil, &jsonRPCError{Code: -32601, Message: "method not found"}
	}
}
