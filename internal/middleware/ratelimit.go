package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ckdash-git/MindSync-AI-Server/pkg/response"
)

// RateLimitConfig holds per-endpoint-type rate limit settings.
type RateLimitConfig struct {
	AuthRPM    int // requests per minute for auth endpoints
	ChatRPM    int // requests per minute for chat endpoints
	StreamRPM  int // requests per minute for streaming endpoints
	CouncilRPM int // requests per minute for council endpoints
}

// RateLimiter implements per-user/IP rate limiting with token bucket algorithm.
type RateLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*bucket
	config  RateLimitConfig
}

type bucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		config:  cfg,
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Handler returns middleware that applies rate limiting based on endpoint type.
func (rl *RateLimiter) Handler(endpointType string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := rl.getKey(r, endpointType)
			rpm := rl.getRPM(endpointType)

			if !rl.allow(key, rpm) {
				retryAfter := 60.0 / float64(rpm)
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter)))
				response.TooManyRequests(w, "rate limit exceeded, try again later")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) allow(key string, rpm int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, exists := rl.buckets[key]
	if !exists {
		b = &bucket{
			tokens:     float64(rpm),
			maxTokens:  float64(rpm),
			refillRate: float64(rpm) / 60.0,
			lastRefill: time.Now(),
		}
		rl.buckets[key] = b
	}

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastRefill = now

	// Try to consume a token
	if b.tokens < 1 {
		return false
	}

	b.tokens--
	return true
}

func (rl *RateLimiter) getKey(r *http.Request, endpointType string) string {
	// Try to get user ID from context first (authenticated)
	if claims, ok := GetClaims(r.Context()); ok {
		return "user:" + claims.UserID.String() + ":" + endpointType
	}

	// Fall back to IP
	ip := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = strings.Split(forwarded, ",")[0]
	}

	return "ip:" + strings.TrimSpace(ip) + ":" + endpointType
}

func (rl *RateLimiter) getRPM(endpointType string) int {
	switch endpointType {
	case "auth":
		return rl.config.AuthRPM
	case "stream":
		return rl.config.StreamRPM
	case "council":
		return rl.config.CouncilRPM
	default:
		return rl.config.ChatRPM
	}
}

// cleanup removes stale buckets every 5 minutes.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, b := range rl.buckets {
			if now.Sub(b.lastRefill) > 10*time.Minute {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}
