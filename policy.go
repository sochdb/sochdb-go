// Copyright 2025 Sushanth (https://github.com/sushanthpy)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package toondb provides policy and safety hooks for agent operations.
//
// The PolicyEngine wraps database operations with policy enforcement,
// enabling pre-write validation, post-read filtering, rate limiting,
// and audit logging for AI agent safety and compliance scenarios.
//
// Example:
//
//	db, _ := toondb.Open("./data")
//	policy := toondb.NewPolicyEngine(db)
//
//	// Block writes to system keys
//	policy.BeforeWrite("system/*", func(ctx *PolicyContext) PolicyAction {
//	    if ctx.AgentID != "" {
//	        return PolicyDeny
//	    }
//	    return PolicyAllow
//	})
//
//	// Redact sensitive data on read
//	policy.AfterRead("users/*/email", func(ctx *PolicyContext) PolicyAction {
//	    if ctx.Get("redact_pii") == "true" {
//	        ctx.ModifiedValue = []byte("[REDACTED]")
//	        return PolicyModify
//	    }
//	    return PolicyAllow
//	})
//
//	// Rate limit writes per agent
//	policy.AddRateLimit("write", 100, "agent_id")
//
//	// Use policy-wrapped operations
//	err := policy.Put([]byte("users/alice"), []byte("data"), map[string]string{
//	    "agent_id": "agent_001",
//	})

package toondb

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

// PolicyAction represents the action to take when a policy is triggered.
type PolicyAction int

const (
	// PolicyAllow allows the operation.
	PolicyAllow PolicyAction = iota
	// PolicyDeny blocks the operation.
	PolicyDeny
	// PolicyModify allows with modifications (check ModifiedValue in context).
	PolicyModify
	// PolicyLog allows but logs the operation.
	PolicyLog
)

// PolicyTrigger indicates when a policy is evaluated.
type PolicyTrigger int

const (
	BeforeRead PolicyTrigger = iota
	AfterRead
	BeforeWrite
	AfterWrite
	BeforeDelete
	AfterDelete
)

// PolicyContext provides context for policy evaluation.
type PolicyContext struct {
	Operation     string
	Key           []byte
	Value         []byte
	ModifiedValue []byte
	AgentID       string
	SessionID     string
	Timestamp     time.Time
	Custom        map[string]string
}

// Get returns a custom context value.
func (c *PolicyContext) Get(key string) string {
	if c.Custom == nil {
		return ""
	}
	return c.Custom[key]
}

// PolicyHandler is a function that evaluates a policy.
type PolicyHandler func(ctx *PolicyContext) PolicyAction

// PatternPolicy applies to keys matching a glob pattern.
type PatternPolicy struct {
	Pattern string
	Trigger PolicyTrigger
	Handler PolicyHandler
	regex   *regexp.Regexp
}

func newPatternPolicy(pattern string, trigger PolicyTrigger, handler PolicyHandler) *PatternPolicy {
	// Convert glob pattern to regex
	regex := pattern
	regex = strings.ReplaceAll(regex, ".", "\\.")
	regex = strings.ReplaceAll(regex, "**", ".*")
	regex = strings.ReplaceAll(regex, "*", "[^/]*")
	regex = "^" + regex + "$"

	return &PatternPolicy{
		Pattern: pattern,
		Trigger: trigger,
		Handler: handler,
		regex:   regexp.MustCompile(regex),
	}
}

func (p *PatternPolicy) matches(key []byte) bool {
	return p.regex.MatchString(string(key))
}

// RateLimiter implements a token bucket rate limiter.
type RateLimiter struct {
	maxPerMinute int
	tokens       int
	lastRefill   time.Time
	mu           sync.Mutex
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(maxPerMinute int) *RateLimiter {
	return &RateLimiter{
		maxPerMinute: maxPerMinute,
		tokens:       maxPerMinute,
		lastRefill:   time.Now(),
	}
}

// TryAcquire attempts to acquire a token. Returns true if allowed.
func (r *RateLimiter) TryAcquire() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastRefill)

	// Refill tokens based on elapsed time
	refill := int(elapsed.Minutes() * float64(r.maxPerMinute))
	if refill > 0 {
		r.tokens = min(r.maxPerMinute, r.tokens+refill)
		r.lastRefill = now
	}

	if r.tokens > 0 {
		r.tokens--
		return true
	}
	return false
}

// AuditEntry represents a logged operation.
type AuditEntry struct {
	Timestamp time.Time
	Operation string
	Key       string
	AgentID   string
	SessionID string
	Result    string
}

// PolicyEngine enforces safety rules on database operations.
type PolicyEngine struct {
	db              *Database
	policies        map[PolicyTrigger][]*PatternPolicy
	rateLimiters    map[string]map[string]*RateLimiter // limiterKey -> scopeKey -> limiter
	rateLimitConfig []rateLimitConfig
	auditLog        []AuditEntry
	auditEnabled    bool
	maxAuditEntries int
	mu              sync.RWMutex
}

type rateLimitConfig struct {
	operation    string
	maxPerMinute int
	scope        string
}

// NewPolicyEngine creates a new policy engine wrapping a database.
func NewPolicyEngine(db *Database) *PolicyEngine {
	return &PolicyEngine{
		db:              db,
		policies:        make(map[PolicyTrigger][]*PatternPolicy),
		rateLimiters:    make(map[string]map[string]*RateLimiter),
		rateLimitConfig: []rateLimitConfig{},
		auditLog:        []AuditEntry{},
		auditEnabled:    false,
		maxAuditEntries: 10000,
	}
}

// BeforeWrite registers a pre-write policy handler.
func (e *PolicyEngine) BeforeWrite(pattern string, handler PolicyHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policies[BeforeWrite] = append(e.policies[BeforeWrite], newPatternPolicy(pattern, BeforeWrite, handler))
}

// AfterWrite registers a post-write policy handler.
func (e *PolicyEngine) AfterWrite(pattern string, handler PolicyHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policies[AfterWrite] = append(e.policies[AfterWrite], newPatternPolicy(pattern, AfterWrite, handler))
}

// BeforeRead registers a pre-read policy handler.
func (e *PolicyEngine) BeforeRead(pattern string, handler PolicyHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policies[BeforeRead] = append(e.policies[BeforeRead], newPatternPolicy(pattern, BeforeRead, handler))
}

// AfterRead registers a post-read policy handler.
func (e *PolicyEngine) AfterRead(pattern string, handler PolicyHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policies[AfterRead] = append(e.policies[AfterRead], newPatternPolicy(pattern, AfterRead, handler))
}

// BeforeDelete registers a pre-delete policy handler.
func (e *PolicyEngine) BeforeDelete(pattern string, handler PolicyHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policies[BeforeDelete] = append(e.policies[BeforeDelete], newPatternPolicy(pattern, BeforeDelete, handler))
}

// AddRateLimit adds a rate limit policy.
// operation: "read", "write", "delete", or "all"
// maxPerMinute: maximum operations per minute
// scope: "global", "agent_id", or "session_id"
func (e *PolicyEngine) AddRateLimit(operation string, maxPerMinute int, scope string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rateLimitConfig = append(e.rateLimitConfig, rateLimitConfig{
		operation:    operation,
		maxPerMinute: maxPerMinute,
		scope:        scope,
	})
}

// EnableAudit enables audit logging.
func (e *PolicyEngine) EnableAudit(maxEntries int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.auditEnabled = true
	e.maxAuditEntries = maxEntries
}

// DisableAudit disables audit logging.
func (e *PolicyEngine) DisableAudit() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.auditEnabled = false
}

// GetAuditLog returns recent audit entries.
func (e *PolicyEngine) GetAuditLog(limit int) []AuditEntry {
	e.mu.RLock()
	defer e.mu.RUnlock()

	start := len(e.auditLog) - limit
	if start < 0 {
		start = 0
	}
	result := make([]AuditEntry, len(e.auditLog)-start)
	copy(result, e.auditLog[start:])
	return result
}

func (e *PolicyEngine) checkRateLimit(operation string, ctx *PolicyContext) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, config := range e.rateLimitConfig {
		if config.operation != operation && config.operation != "all" {
			continue
		}

		scopeKey := "global"
		switch config.scope {
		case "agent_id":
			scopeKey = ctx.AgentID
		case "session_id":
			scopeKey = ctx.SessionID
		default:
			if val := ctx.Get(config.scope); val != "" {
				scopeKey = val
			}
		}

		limiterKey := config.operation + ":" + config.scope
		if e.rateLimiters[limiterKey] == nil {
			e.rateLimiters[limiterKey] = make(map[string]*RateLimiter)
		}
		if e.rateLimiters[limiterKey][scopeKey] == nil {
			e.rateLimiters[limiterKey][scopeKey] = NewRateLimiter(config.maxPerMinute)
		}

		if !e.rateLimiters[limiterKey][scopeKey].TryAcquire() {
			return false
		}
	}
	return true
}

func (e *PolicyEngine) evaluatePolicies(trigger PolicyTrigger, ctx *PolicyContext) PolicyAction {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, policy := range e.policies[trigger] {
		if policy.matches(ctx.Key) {
			action := policy.Handler(ctx)
			if action == PolicyDeny || action == PolicyModify {
				return action
			}
		}
	}
	return PolicyAllow
}

func (e *PolicyEngine) audit(operation string, key []byte, ctx *PolicyContext, result string) {
	if !e.auditEnabled {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.auditLog = append(e.auditLog, AuditEntry{
		Timestamp: time.Now(),
		Operation: operation,
		Key:       string(key),
		AgentID:   ctx.AgentID,
		SessionID: ctx.SessionID,
		Result:    result,
	})

	if len(e.auditLog) > e.maxAuditEntries {
		e.auditLog = e.auditLog[len(e.auditLog)-e.maxAuditEntries:]
	}
}

func (e *PolicyEngine) makeContext(operation string, key, value []byte, custom map[string]string) *PolicyContext {
	ctx := &PolicyContext{
		Operation: operation,
		Key:       key,
		Value:     value,
		Timestamp: time.Now(),
		Custom:    custom,
	}
	if custom != nil {
		ctx.AgentID = custom["agent_id"]
		ctx.SessionID = custom["session_id"]
	}
	return ctx
}

// Put writes a value with policy enforcement.
func (e *PolicyEngine) Put(key, value []byte, context map[string]string) error {
	ctx := e.makeContext("write", key, value, context)

	if !e.checkRateLimit("write", ctx) {
		e.audit("write", key, ctx, "rate_limited")
		return &PolicyViolationError{Message: "Rate limit exceeded"}
	}

	action := e.evaluatePolicies(BeforeWrite, ctx)
	if action == PolicyDeny {
		e.audit("write", key, ctx, "denied")
		return &PolicyViolationError{Message: "Write blocked by policy"}
	}

	writeValue := value
	if action == PolicyModify && ctx.ModifiedValue != nil {
		writeValue = ctx.ModifiedValue
	}

	if err := e.db.Put(key, writeValue); err != nil {
		return err
	}

	ctx.Value = writeValue
	e.evaluatePolicies(AfterWrite, ctx)
	e.audit("write", key, ctx, "allowed")
	return nil
}

// Get reads a value with policy enforcement.
func (e *PolicyEngine) Get(key []byte, context map[string]string) ([]byte, error) {
	ctx := e.makeContext("read", key, nil, context)

	if !e.checkRateLimit("read", ctx) {
		e.audit("read", key, ctx, "rate_limited")
		return nil, &PolicyViolationError{Message: "Rate limit exceeded"}
	}

	action := e.evaluatePolicies(BeforeRead, ctx)
	if action == PolicyDeny {
		e.audit("read", key, ctx, "denied")
		return nil, &PolicyViolationError{Message: "Read blocked by policy"}
	}

	value, err := e.db.Get(key)
	if err != nil {
		return nil, err
	}

	ctx.Value = value
	action = e.evaluatePolicies(AfterRead, ctx)

	if action == PolicyModify && ctx.ModifiedValue != nil {
		value = ctx.ModifiedValue
	} else if action == PolicyDeny {
		e.audit("read", key, ctx, "redacted")
		return nil, nil
	}

	e.audit("read", key, ctx, "allowed")
	return value, nil
}

// Delete removes a value with policy enforcement.
func (e *PolicyEngine) Delete(key []byte, context map[string]string) error {
	ctx := e.makeContext("delete", key, nil, context)

	if !e.checkRateLimit("delete", ctx) {
		e.audit("delete", key, ctx, "rate_limited")
		return &PolicyViolationError{Message: "Rate limit exceeded"}
	}

	action := e.evaluatePolicies(BeforeDelete, ctx)
	if action == PolicyDeny {
		e.audit("delete", key, ctx, "denied")
		return &PolicyViolationError{Message: "Delete blocked by policy"}
	}

	if err := e.db.Delete(key); err != nil {
		return err
	}

	e.audit("delete", key, ctx, "allowed")
	return nil
}

// PolicyViolationError is returned when a policy blocks an operation.
type PolicyViolationError struct {
	Message string
}

func (e *PolicyViolationError) Error() string {
	return e.Message
}

// Helper functions for common policies

// DenyAll returns a policy handler that denies all matching operations.
func DenyAll() PolicyHandler {
	return func(ctx *PolicyContext) PolicyAction {
		return PolicyDeny
	}
}

// AllowAll returns a policy handler that allows all matching operations.
func AllowAll() PolicyHandler {
	return func(ctx *PolicyContext) PolicyAction {
		return PolicyAllow
	}
}

// RequireAgentID returns a policy handler that requires an agent_id in context.
func RequireAgentID() PolicyHandler {
	return func(ctx *PolicyContext) PolicyAction {
		if ctx.AgentID != "" {
			return PolicyAllow
		}
		return PolicyDeny
	}
}

// RedactValue returns a policy handler that replaces values with a redaction marker.
func RedactValue(replacement []byte) PolicyHandler {
	return func(ctx *PolicyContext) PolicyAction {
		ctx.ModifiedValue = replacement
		return PolicyModify
	}
}
