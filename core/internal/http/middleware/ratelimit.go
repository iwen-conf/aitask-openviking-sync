package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/identity"
)

type RateLimitOptions struct {
	Enabled         bool
	Store           redis.Cmdable
	Capacity        int
	RefillPerSecond float64
	KeyPrefix       string
}

var tokenBucketScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local refill_per_sec = tonumber(ARGV[2])
local capacity = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])
local ttl_ms = tonumber(ARGV[5])

local values = redis.call("HMGET", KEYS[1], "tokens", "ts")
local tokens = tonumber(values[1])
local ts = tonumber(values[2])
if tokens == nil then tokens = capacity end
if ts == nil then ts = now end

local delta = now - ts
if delta < 0 then delta = 0 end
tokens = math.min(capacity, tokens + (delta * refill_per_sec / 1000))

local allowed = 0
if tokens >= requested then
  allowed = 1
  tokens = tokens - requested
end

redis.call("HMSET", KEYS[1], "tokens", tokens, "ts", now)
redis.call("PEXPIRE", KEYS[1], ttl_ms)
return {allowed, tokens}
`)

func RateLimitTokenBucket(opts RateLimitOptions) gin.HandlerFunc {
	if !opts.Enabled || opts.Store == nil || opts.Capacity <= 0 || opts.RefillPerSecond <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	keyPrefix := strings.TrimSpace(opts.KeyPrefix)
	if keyPrefix == "" {
		keyPrefix = "rl:token"
	}
	retryAfterSeconds := int(math.Ceil(1.0 / opts.RefillPerSecond))
	if retryAfterSeconds < 1 {
		retryAfterSeconds = 1
	}

	return func(c *gin.Context) {
		identityKey := resolveRateLimitIdentity(c)
		bucketKey := buildBucketKey(keyPrefix, identityKey)
		nowMillis := time.Now().UTC().UnixMilli()
		ttlMillis := int64(math.Ceil((float64(opts.Capacity)/opts.RefillPerSecond)*2000 + 1000))

		rawResult, err := tokenBucketScript.Run(
			c.Request.Context(),
			opts.Store,
			[]string{bucketKey},
			nowMillis,
			opts.RefillPerSecond,
			opts.Capacity,
			1,
			ttlMillis,
		).Result()
		if err != nil {
			// Redis fail-open: request continues to keep service available.
			c.Next()
			return
		}

		allowed, remaining := parseRateLimitResult(rawResult)
		c.Header("X-RateLimit-Limit", strconv.Itoa(opts.Capacity))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if allowed {
			c.Next()
			return
		}

		c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
		writeError(c, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests", true, map[string]any{
			"retryAfterSeconds": retryAfterSeconds,
		})
	}
}

func resolveRateLimitIdentity(c *gin.Context) string {
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if parts := strings.SplitN(header, " ", 2); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && strings.TrimSpace(parts[1]) != "" {
		return "agent_token:" + strings.TrimSpace(parts[1])
	}
	if raw, ok := c.Get(identityContextKey); ok {
		if value, ok := raw.(identity.Identity); ok {
			if value.IsAgent() {
				return "agent:" + strings.TrimSpace(value.Agent.AgentID)
			}
			if value.IsOperator() {
				return "operator:" + strings.TrimSpace(value.OperatorLabel)
			}
			if value.IsSystem() {
				return "system"
			}
		}
	}
	operator := strings.TrimSpace(c.GetHeader("X-Operator-Label"))
	if operator != "" {
		return "operator:" + operator
	}
	return "ip:" + strings.TrimSpace(c.ClientIP())
}

type identityWrapper interface {
	rateLimitIdentity() string
}

func buildBucketKey(prefix string, identity string) string {
	hash := sha256.Sum256([]byte(identity))
	return prefix + ":" + hex.EncodeToString(hash[:16])
}

func parseRateLimitResult(value any) (allowed bool, remaining int) {
	items, ok := value.([]any)
	if !ok || len(items) < 2 {
		return true, 0
	}
	allowFloat, ok := items[0].(int64)
	if ok {
		allowed = allowFloat == 1
	} else {
		allowed = true
	}
	switch v := items[1].(type) {
	case int64:
		remaining = int(v)
	case float64:
		remaining = int(v)
	default:
		remaining = 0
	}
	return allowed, remaining
}
