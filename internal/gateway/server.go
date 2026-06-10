package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yuaiccc/go-agent-gateway-demo/internal/agent"
	"github.com/yuaiccc/go-agent-gateway-demo/internal/session"
	"github.com/yuaiccc/go-agent-gateway-demo/internal/tenant"
	"github.com/yuaiccc/go-agent-gateway-demo/internal/tool"
)

type Server struct {
	Tenants  *tenant.Store
	Sessions *session.Store
	Tools    *tool.Registry
	Agent    *agent.Service
}

func NewServer() *Server {
	tools := tool.NewRegistry()
	return &Server{
		Tenants:  tenant.NewStore(),
		Sessions: session.NewStore(),
		Tools:    tools,
		Agent:    agent.NewService(tools),
	}
}

func (s *Server) Router() *gin.Engine {
	r := gin.Default()

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.GET("/api/tenants", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"tenants": s.Tenants.List()})
	})

	r.PATCH("/api/tenants/:tenantID/model", s.updateTenantModel)
	r.POST("/api/agent/stream", s.streamAgent)

	mcp := r.Group("/mcp")
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
	if req.SessionID == "" {
		req.SessionID = fmt.Sprintf("sess-%d", time.Now().UnixMilli())
	}

	cfg, ok := s.Tenants.Get(req.TenantID)
	if !ok || !cfg.Active {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found or inactive"})
		return
	}
	if err := s.Sessions.ValidateOwner(req.SessionID, req.TenantID, req.UserID); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	s.Sessions.GetOrCreate(req.TenantID, req.UserID, req.SessionID)
	_, _ = s.Sessions.Append(req.SessionID, "user", req.Message)

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
		_, _ = s.Sessions.Append(req.SessionID, "assistant", finalAnswer)
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
