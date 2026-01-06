// Copyright 2025 Sushanth (https://github.com/sushanthpy)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package toondb provides tool routing for multi-agent scenarios.
//
// The ToolRouter routes tool calls to appropriate agents based on
// capabilities, load balancing, and routing strategies. This enables
// multi-agent architectures where specialized agents handle different
// tool domains.
//
// Example:
//
//	db, _ := toondb.Open("./data")
//	registry := toondb.NewAgentRegistry(db)
//	router := toondb.NewToolRouter(registry)
//
//	// Register agents with capabilities
//	registry.RegisterAgent("code_agent", []ToolCategory{CategoryCode, CategoryGit},
//	    WithEndpoint("http://localhost:8001/invoke"))
//
//	registry.RegisterAgent("search_agent", []ToolCategory{CategorySearch},
//	    WithHandler(func(tool string, args map[string]any) (any, error) {
//	        return map[string]any{"results": []string{}}, nil
//	    }))
//
//	// Register tools
//	router.RegisterTool(Tool{
//	    Name:        "search_code",
//	    Description: "Search codebase",
//	    Category:    CategoryCode,
//	})
//
//	// Route a tool call
//	result := router.Route("search_code", map[string]any{"query": "auth"},
//	    WithSessionID("sess_001"))

package toondb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// ToolCategory represents standard tool categories for routing.
type ToolCategory string

const (
	CategoryCode     ToolCategory = "code"
	CategorySearch   ToolCategory = "search"
	CategoryDatabase ToolCategory = "database"
	CategoryWeb      ToolCategory = "web"
	CategoryFile     ToolCategory = "file"
	CategoryGit      ToolCategory = "git"
	CategoryShell    ToolCategory = "shell"
	CategoryEmail    ToolCategory = "email"
	CategoryCalendar ToolCategory = "calendar"
	CategoryMemory   ToolCategory = "memory"
	CategoryVector   ToolCategory = "vector"
	CategoryGraph    ToolCategory = "graph"
	CategoryCustom   ToolCategory = "custom"
)

// RoutingStrategy determines how to select among capable agents.
type RoutingStrategy int

const (
	StrategyRoundRobin RoutingStrategy = iota
	StrategyRandom
	StrategyLeastLoaded
	StrategySticky
	StrategyPriority
	StrategyFastest
)

// AgentStatus represents agent availability.
type AgentStatus string

const (
	StatusAvailable AgentStatus = "available"
	StatusBusy      AgentStatus = "busy"
	StatusOffline   AgentStatus = "offline"
	StatusDegraded  AgentStatus = "degraded"
)

// Tool defines a routable tool.
type Tool struct {
	Name                 string
	Description          string
	Category             ToolCategory
	Schema               map[string]any
	RequiredCapabilities []ToolCategory
	TimeoutSeconds       float64
	Retries              int
	Metadata             map[string]any
}

// ToolHandler is a function that handles tool invocations.
type ToolHandler func(toolName string, args map[string]any) (any, error)

// Agent represents an agent that can handle tools.
type Agent struct {
	AgentID       string
	Capabilities  []ToolCategory
	Endpoint      string
	Handler       ToolHandler
	Priority      int
	MaxConcurrent int
	Metadata      map[string]any

	// Runtime state
	Status       AgentStatus
	CurrentLoad  int
	TotalCalls   int
	TotalLatency time.Duration
	LastSuccess  time.Time
	LastFailure  time.Time
}

// RouteResult holds the result of a tool routing decision.
type RouteResult struct {
	AgentID     string
	ToolName    string
	Result      any
	LatencyMs   float64
	Success     bool
	Error       string
	RetriesUsed int
}

// RoutingContext provides context for routing decisions.
type RoutingContext struct {
	SessionID       string
	UserID          string
	Priority        int
	TimeoutOverride *float64
	PreferredAgent  string
	ExcludedAgents  []string
	Custom          map[string]any
}

// AgentOption is a functional option for agent registration.
type AgentOption func(*Agent)

// WithEndpoint sets the HTTP endpoint for a remote agent.
func WithEndpoint(endpoint string) AgentOption {
	return func(a *Agent) {
		a.Endpoint = endpoint
	}
}

// WithHandler sets the local handler function for an in-process agent.
func WithHandler(handler ToolHandler) AgentOption {
	return func(a *Agent) {
		a.Handler = handler
	}
}

// WithPriority sets the routing priority (higher = preferred).
func WithPriority(priority int) AgentOption {
	return func(a *Agent) {
		a.Priority = priority
	}
}

// WithMaxConcurrent sets the maximum concurrent tool calls.
func WithMaxConcurrent(max int) AgentOption {
	return func(a *Agent) {
		a.MaxConcurrent = max
	}
}

// RoutingOption is a functional option for routing calls.
type RoutingOption func(*RoutingContext)

// WithSessionID sets the session ID for sticky routing.
func WithSessionID(sessionID string) RoutingOption {
	return func(ctx *RoutingContext) {
		ctx.SessionID = sessionID
	}
}

// WithPreferredAgent sets a preferred agent for routing.
func WithPreferredAgent(agentID string) RoutingOption {
	return func(ctx *RoutingContext) {
		ctx.PreferredAgent = agentID
	}
}

// WithExcludedAgents sets agents to exclude from routing.
func WithExcludedAgents(agentIDs ...string) RoutingOption {
	return func(ctx *RoutingContext) {
		ctx.ExcludedAgents = agentIDs
	}
}

// WithTimeout sets a timeout override for the call.
func WithTimeout(seconds float64) RoutingOption {
	return func(ctx *RoutingContext) {
		ctx.TimeoutOverride = &seconds
	}
}

// AgentRegistry manages agent registrations.
type AgentRegistry struct {
	db     *Database
	agents map[string]*Agent
	mu     sync.RWMutex
}

const agentPrefix = "/_routing/agents/"

// NewAgentRegistry creates a new agent registry.
func NewAgentRegistry(db *Database) *AgentRegistry {
	r := &AgentRegistry{
		db:     db,
		agents: make(map[string]*Agent),
	}
	r.loadAgents()
	return r
}

func (r *AgentRegistry) loadAgents() {
	results, err := r.db.Scan(agentPrefix)
	if err != nil {
		return
	}

	for _, kv := range results {
		var data map[string]any
		if err := json.Unmarshal(kv.Value, &data); err != nil {
			continue
		}

		caps := []ToolCategory{}
		if capsList, ok := data["capabilities"].([]any); ok {
			for _, c := range capsList {
				if s, ok := c.(string); ok {
					caps = append(caps, ToolCategory(s))
				}
			}
		}

		agent := &Agent{
			AgentID:       data["agent_id"].(string),
			Capabilities:  caps,
			Priority:      100,
			MaxConcurrent: 10,
			Status:        StatusAvailable,
		}
		if ep, ok := data["endpoint"].(string); ok {
			agent.Endpoint = ep
		}
		if p, ok := data["priority"].(float64); ok {
			agent.Priority = int(p)
		}
		if mc, ok := data["max_concurrent"].(float64); ok {
			agent.MaxConcurrent = int(mc)
		}

		r.agents[agent.AgentID] = agent
	}
}

// RegisterAgent registers an agent with capabilities.
func (r *AgentRegistry) RegisterAgent(agentID string, capabilities []ToolCategory, opts ...AgentOption) *Agent {
	agent := &Agent{
		AgentID:       agentID,
		Capabilities:  capabilities,
		Priority:      100,
		MaxConcurrent: 10,
		Metadata:      make(map[string]any),
		Status:        StatusAvailable,
	}

	for _, opt := range opts {
		opt(agent)
	}

	r.mu.Lock()
	r.agents[agentID] = agent

	// Persist to database
	data := map[string]any{
		"agent_id":       agentID,
		"capabilities":   capabilities,
		"endpoint":       agent.Endpoint,
		"priority":       agent.Priority,
		"max_concurrent": agent.MaxConcurrent,
		"metadata":       agent.Metadata,
	}
	jsonData, _ := json.Marshal(data)
	r.db.Put([]byte(agentPrefix+agentID), jsonData)
	r.mu.Unlock()

	return agent
}

// UnregisterAgent removes an agent registration.
func (r *AgentRegistry) UnregisterAgent(agentID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[agentID]; exists {
		delete(r.agents, agentID)
		r.db.Delete([]byte(agentPrefix + agentID))
		return true
	}
	return false
}

// GetAgent returns an agent by ID.
func (r *AgentRegistry) GetAgent(agentID string) *Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.agents[agentID]
}

// ListAgents returns all registered agents.
func (r *AgentRegistry) ListAgents() []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*Agent, 0, len(r.agents))
	for _, a := range r.agents {
		agents = append(agents, a)
	}
	return agents
}

// FindCapableAgents finds agents that can handle the required categories.
func (r *AgentRegistry) FindCapableAgents(required []ToolCategory, exclude []string) []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	excludeSet := make(map[string]bool)
	for _, id := range exclude {
		excludeSet[id] = true
	}

	capable := []*Agent{}
	for _, agent := range r.agents {
		if excludeSet[agent.AgentID] {
			continue
		}
		if agent.Status == StatusOffline {
			continue
		}

		// Check if agent has all required capabilities
		agentCaps := make(map[ToolCategory]bool)
		for _, c := range agent.Capabilities {
			agentCaps[c] = true
		}

		hasAll := true
		for _, req := range required {
			if !agentCaps[req] {
				hasAll = false
				break
			}
		}
		if hasAll {
			capable = append(capable, agent)
		}
	}
	return capable
}

// UpdateAgentStatus updates an agent's status.
func (r *AgentRegistry) UpdateAgentStatus(agentID string, status AgentStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if agent, ok := r.agents[agentID]; ok {
		agent.Status = status
	}
}

// RecordCall records a tool call result for an agent.
func (r *AgentRegistry) RecordCall(agentID string, latency time.Duration, success bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if agent, ok := r.agents[agentID]; ok {
		agent.TotalCalls++
		agent.TotalLatency += latency
		if success {
			agent.LastSuccess = time.Now()
		} else {
			agent.LastFailure = time.Now()
		}
	}
}

// ToolRouter routes tool calls to appropriate agents.
type ToolRouter struct {
	registry        *AgentRegistry
	db              *Database
	defaultStrategy RoutingStrategy
	tools           map[string]*Tool
	roundRobinIdx   map[string]int
	sessionAffinity map[string]string
	mu              sync.RWMutex
}

const toolPrefix = "/_routing/tools/"

// NewToolRouter creates a new tool router.
func NewToolRouter(registry *AgentRegistry) *ToolRouter {
	r := &ToolRouter{
		registry:        registry,
		db:              registry.db,
		defaultStrategy: StrategyPriority,
		tools:           make(map[string]*Tool),
		roundRobinIdx:   make(map[string]int),
		sessionAffinity: make(map[string]string),
	}
	r.loadTools()
	return r
}

// WithDefaultStrategy sets the default routing strategy.
func (r *ToolRouter) WithDefaultStrategy(strategy RoutingStrategy) *ToolRouter {
	r.defaultStrategy = strategy
	return r
}

func (r *ToolRouter) loadTools() {
	results, err := r.db.Scan(toolPrefix)
	if err != nil {
		return
	}

	for _, kv := range results {
		var data map[string]any
		if err := json.Unmarshal(kv.Value, &data); err != nil {
			continue
		}

		tool := &Tool{
			Name:           data["name"].(string),
			Description:    data["description"].(string),
			Category:       ToolCategory(data["category"].(string)),
			TimeoutSeconds: 30.0,
			Retries:        1,
		}
		if schema, ok := data["schema"].(map[string]any); ok {
			tool.Schema = schema
		}
		if timeout, ok := data["timeout_seconds"].(float64); ok {
			tool.TimeoutSeconds = timeout
		}
		if retries, ok := data["retries"].(float64); ok {
			tool.Retries = int(retries)
		}

		r.tools[tool.Name] = tool
	}
}

// RegisterTool registers a tool for routing.
func (r *ToolRouter) RegisterTool(tool Tool) *Tool {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools[tool.Name] = &tool

	// Persist to database
	data := map[string]any{
		"name":                  tool.Name,
		"description":           tool.Description,
		"category":              tool.Category,
		"schema":                tool.Schema,
		"required_capabilities": tool.RequiredCapabilities,
		"timeout_seconds":       tool.TimeoutSeconds,
		"retries":               tool.Retries,
		"metadata":              tool.Metadata,
	}
	jsonData, _ := json.Marshal(data)
	r.db.Put([]byte(toolPrefix+tool.Name), jsonData)

	return &tool
}

// UnregisterTool removes a tool registration.
func (r *ToolRouter) UnregisterTool(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; exists {
		delete(r.tools, name)
		r.db.Delete([]byte(toolPrefix + name))
		return true
	}
	return false
}

// GetTool returns a tool by name.
func (r *ToolRouter) GetTool(name string) *Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// ListTools returns all registered tools.
func (r *ToolRouter) ListTools() []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]*Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// Route routes a tool call to the best agent.
func (r *ToolRouter) Route(toolName string, args map[string]any, opts ...RoutingOption) RouteResult {
	ctx := &RoutingContext{Priority: 100}
	for _, opt := range opts {
		opt(ctx)
	}

	r.mu.RLock()
	tool, exists := r.tools[toolName]
	r.mu.RUnlock()

	if !exists {
		return RouteResult{
			ToolName: toolName,
			Success:  false,
			Error:    fmt.Sprintf("Unknown tool: %s", toolName),
		}
	}

	// Determine required capabilities
	required := tool.RequiredCapabilities
	if len(required) == 0 {
		required = []ToolCategory{tool.Category}
	}

	// Find capable agents
	capable := r.registry.FindCapableAgents(required, ctx.ExcludedAgents)
	if len(capable) == 0 {
		return RouteResult{
			ToolName: toolName,
			Success:  false,
			Error:    fmt.Sprintf("No capable agents for tool '%s'", toolName),
		}
	}

	// Select agent using strategy
	agent := r.selectAgent(capable, r.defaultStrategy, ctx)

	// Execute with retries
	timeout := tool.TimeoutSeconds
	if ctx.TimeoutOverride != nil {
		timeout = *ctx.TimeoutOverride
	}
	retries := tool.Retries
	var lastError string

	for attempt := 0; attempt <= retries; attempt++ {
		start := time.Now()
		result, err := r.invokeAgent(agent, tool, args, timeout)
		latency := time.Since(start)
		latencyMs := float64(latency.Milliseconds())

		if err == nil {
			r.registry.RecordCall(agent.AgentID, latency, true)

			// Update session affinity
			if ctx.SessionID != "" {
				r.mu.Lock()
				r.sessionAffinity[ctx.SessionID] = agent.AgentID
				r.mu.Unlock()
			}

			return RouteResult{
				AgentID:     agent.AgentID,
				ToolName:    toolName,
				Result:      result,
				LatencyMs:   latencyMs,
				Success:     true,
				RetriesUsed: attempt,
			}
		}

		r.registry.RecordCall(agent.AgentID, latency, false)
		lastError = err.Error()

		// Try next capable agent on failure
		var remaining []*Agent
		for _, a := range capable {
			if a.AgentID != agent.AgentID {
				remaining = append(remaining, a)
			}
		}
		if len(remaining) > 0 {
			capable = remaining
			agent = r.selectAgent(capable, r.defaultStrategy, ctx)
		}
	}

	return RouteResult{
		AgentID:     agent.AgentID,
		ToolName:    toolName,
		Success:     false,
		Error:       lastError,
		RetriesUsed: retries,
	}
}

func (r *ToolRouter) selectAgent(capable []*Agent, strategy RoutingStrategy, ctx *RoutingContext) *Agent {
	if len(capable) == 0 {
		return nil
	}

	// Preferred agent override
	if ctx.PreferredAgent != "" {
		for _, agent := range capable {
			if agent.AgentID == ctx.PreferredAgent {
				return agent
			}
		}
	}

	// Session affinity (sticky routing)
	if strategy == StrategySticky && ctx.SessionID != "" {
		r.mu.RLock()
		prevAgent := r.sessionAffinity[ctx.SessionID]
		r.mu.RUnlock()
		for _, agent := range capable {
			if agent.AgentID == prevAgent {
				return agent
			}
		}
	}

	switch strategy {
	case StrategyRoundRobin:
		r.mu.Lock()
		key := ""
		for _, a := range capable {
			key += a.AgentID + ","
		}
		idx := r.roundRobinIdx[key] % len(capable)
		r.roundRobinIdx[key]++
		r.mu.Unlock()
		return capable[idx]

	case StrategyRandom:
		return capable[rand.Intn(len(capable))]

	case StrategyLeastLoaded:
		best := capable[0]
		for _, a := range capable[1:] {
			if a.CurrentLoad < best.CurrentLoad {
				best = a
			}
		}
		return best

	case StrategyPriority:
		best := capable[0]
		for _, a := range capable[1:] {
			if a.Priority > best.Priority || (a.Priority == best.Priority && a.CurrentLoad < best.CurrentLoad) {
				best = a
			}
		}
		return best

	case StrategyFastest:
		var best *Agent
		var bestAvg time.Duration = 1<<63 - 1
		for _, a := range capable {
			if a.TotalCalls > 0 {
				avg := a.TotalLatency / time.Duration(a.TotalCalls)
				if avg < bestAvg {
					bestAvg = avg
					best = a
				}
			}
		}
		if best != nil {
			return best
		}
		return capable[0]

	default:
		return capable[0]
	}
}

func (r *ToolRouter) invokeAgent(agent *Agent, tool *Tool, args map[string]any, timeout float64) (any, error) {
	agent.CurrentLoad++
	defer func() { agent.CurrentLoad-- }()

	if agent.Handler != nil {
		// Local function handler
		return agent.Handler(tool.Name, args)
	}

	if agent.Endpoint != "" {
		// Remote HTTP invocation
		requestData, _ := json.Marshal(map[string]any{
			"tool":     tool.Name,
			"args":     args,
			"metadata": tool.Metadata,
		})

		client := &http.Client{Timeout: time.Duration(timeout * float64(time.Second))}
		resp, err := client.Post(agent.Endpoint, "application/json", bytes.NewReader(requestData))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		var result any
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}
		return result, nil
	}

	return nil, fmt.Errorf("agent %s has no handler or endpoint", agent.AgentID)
}

// ToolDispatcher provides a high-level API for multi-agent tool orchestration.
type ToolDispatcher struct {
	db       *Database
	registry *AgentRegistry
	router   *ToolRouter
}

// NewToolDispatcher creates a new tool dispatcher.
func NewToolDispatcher(db *Database) *ToolDispatcher {
	registry := NewAgentRegistry(db)
	router := NewToolRouter(registry)
	return &ToolDispatcher{
		db:       db,
		registry: registry,
		router:   router,
	}
}

// Registry returns the agent registry.
func (d *ToolDispatcher) Registry() *AgentRegistry {
	return d.registry
}

// Router returns the tool router.
func (d *ToolDispatcher) Router() *ToolRouter {
	return d.router
}

// RegisterLocalAgent registers a local (in-process) agent.
func (d *ToolDispatcher) RegisterLocalAgent(agentID string, capabilities []ToolCategory, handler ToolHandler, priority int) *Agent {
	return d.registry.RegisterAgent(agentID, capabilities, WithHandler(handler), WithPriority(priority))
}

// RegisterRemoteAgent registers a remote (HTTP) agent.
func (d *ToolDispatcher) RegisterRemoteAgent(agentID string, capabilities []ToolCategory, endpoint string, priority int) *Agent {
	return d.registry.RegisterAgent(agentID, capabilities, WithEndpoint(endpoint), WithPriority(priority))
}

// RegisterTool registers a tool for routing.
func (d *ToolDispatcher) RegisterTool(name, description string, category ToolCategory, schema map[string]any, timeout float64) *Tool {
	return d.router.RegisterTool(Tool{
		Name:           name,
		Description:    description,
		Category:       category,
		Schema:         schema,
		TimeoutSeconds: timeout,
	})
}

// Invoke invokes a tool with automatic routing.
func (d *ToolDispatcher) Invoke(toolName string, args map[string]any, opts ...RoutingOption) RouteResult {
	return d.router.Route(toolName, args, opts...)
}

// ListAgents returns all registered agents with their status.
func (d *ToolDispatcher) ListAgents() []map[string]any {
	agents := d.registry.ListAgents()
	result := make([]map[string]any, len(agents))
	for i, a := range agents {
		var avgLatency *float64
		if a.TotalCalls > 0 {
			avg := float64(a.TotalLatency.Milliseconds()) / float64(a.TotalCalls)
			avgLatency = &avg
		}
		result[i] = map[string]any{
			"agent_id":       a.AgentID,
			"capabilities":   a.Capabilities,
			"status":         a.Status,
			"priority":       a.Priority,
			"current_load":   a.CurrentLoad,
			"total_calls":    a.TotalCalls,
			"avg_latency_ms": avgLatency,
			"has_endpoint":   a.Endpoint != "",
			"has_handler":    a.Handler != nil,
		}
	}
	return result
}

// ListTools returns all registered tools.
func (d *ToolDispatcher) ListTools() []map[string]any {
	tools := d.router.ListTools()
	result := make([]map[string]any, len(tools))
	for i, t := range tools {
		result[i] = map[string]any{
			"name":            t.Name,
			"description":     t.Description,
			"category":        t.Category,
			"schema":          t.Schema,
			"timeout_seconds": t.TimeoutSeconds,
			"retries":         t.Retries,
		}
	}
	return result
}
