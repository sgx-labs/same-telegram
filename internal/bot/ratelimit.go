package bot

import (
	"log"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// rateLimiter enforces per-chat message rate limits to avoid Telegram 429 errors.
// Limit: 1 message per second per chat, burst of 3.
type rateLimiter struct {
	mu         sync.Mutex
	buckets    map[int64]*tokenBucket
	refillRate float64 // default refill rate for new buckets
	maxTokens  float64 // default max tokens for new buckets
}

// tokenBucket implements a simple token bucket rate limiter.
type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

func newRateLimiter() *rateLimiter {
	return newRateLimiterWithConfig(1.0, 3)
}

// newPublicRateLimiter creates a stricter rate limiter for public mode.
func newPublicRateLimiter() *rateLimiter {
	return newRateLimiterWithConfig(1.0/3.0, 2) // 1 token per 3 seconds, burst 2
}

func newRateLimiterWithConfig(refillRate float64, maxTokens float64) *rateLimiter {
	return &rateLimiter{
		buckets:    make(map[int64]*tokenBucket),
		refillRate: refillRate,
		maxTokens:  maxTokens,
	}
}

func (rl *rateLimiter) getBucket(chatID int64) *tokenBucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[chatID]
	if !ok {
		b = &tokenBucket{
			tokens:     rl.maxTokens,
			maxTokens:  rl.maxTokens,
			refillRate: rl.refillRate,
			lastRefill: time.Now(),
		}
		rl.buckets[chatID] = b
	}
	return b
}

// consume attempts to consume a token from the bucket for the given chat.
// If the bucket is empty, it still allows the send but logs a warning.
// This avoids blocking the update goroutine while still tracking rate usage.
func (rl *rateLimiter) consume(chatID int64, logger *log.Logger) {
	b := rl.getBucket(chatID)
	b.refill()
	if b.tokens >= 1 {
		b.tokens--
		return
	}
	// Bucket empty: allow the send anyway but warn (non-blocking).
	b.tokens = 0
	if logger != nil {
		logger.Printf("Rate limiter: outbound bucket empty for chat %d, sending anyway", chatID)
	}
}

// wait is kept for backward compatibility but now delegates to non-blocking consume.
func (rl *rateLimiter) wait(chatID int64) {
	rl.consume(chatID, nil)
}

// inboundLimiter enforces per-user inbound message rate limits.
// If a user exceeds the rate, their messages are silently dropped.
type inboundLimiter struct {
	mu      sync.Mutex
	buckets map[int64]*tokenBucket
	config  inboundConfig
}

type inboundConfig struct {
	refillRate float64
	maxTokens  float64
}

func newInboundLimiter(refillRate float64, maxTokens float64) *inboundLimiter {
	return &inboundLimiter{
		buckets: make(map[int64]*tokenBucket),
		config: inboundConfig{
			refillRate: refillRate,
			maxTokens:  maxTokens,
		},
	}
}

// allow checks whether a message from this user should be processed.
// Returns true if allowed, false if the message should be silently dropped.
func (il *inboundLimiter) allow(userID int64) bool {
	il.mu.Lock()
	defer il.mu.Unlock()

	b, ok := il.buckets[userID]
	if !ok {
		b = &tokenBucket{
			tokens:     il.config.maxTokens,
			maxTokens:  il.config.maxTokens,
			refillRate: il.config.refillRate,
			lastRefill: time.Now(),
		}
		il.buckets[userID] = b
	}

	b.refill()
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

func (b *tokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastRefill = now
}

// parseRetryAfter extracts the retry_after duration from a Telegram API error.
// Returns 0 if the error is not a 429 rate limit error.
func parseRetryAfter(err error) time.Duration {
	if err == nil {
		return 0
	}
	if apiErr, ok := err.(*tgbotapi.Error); ok && apiErr.RetryAfter > 0 {
		return time.Duration(apiErr.RetryAfter) * time.Second
	}
	return 0
}

// sendWithRetry sends a Telegram message with rate limiting and 429 retry logic.
func (b *Bot) sendWithRetry(msg tgbotapi.MessageConfig) (tgbotapi.Message, error) {
	b.limiter.consume(msg.ChatID, b.logger)

	resp, err := b.api.Send(msg)
	if err != nil {
		if retryAfter := parseRetryAfter(err); retryAfter > 0 {
			b.logger.Printf("Rate limited by Telegram (chat %d), waiting %v", msg.ChatID, retryAfter)
			time.Sleep(retryAfter)
			return b.api.Send(msg)
		}
	}
	return resp, err
}
