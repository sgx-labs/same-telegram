package bot

import (
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// rateLimiter enforces per-chat message rate limits to avoid Telegram 429 errors.
// Limit: 1 message per second per chat, burst of 3.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[int64]*tokenBucket
}

// tokenBucket implements a simple token bucket rate limiter.
type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		buckets: make(map[int64]*tokenBucket),
	}
}

func (rl *rateLimiter) getBucket(chatID int64) *tokenBucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[chatID]
	if !ok {
		b = &tokenBucket{
			tokens:     3,
			maxTokens:  3,
			refillRate: 1, // 1 token per second
			lastRefill: time.Now(),
		}
		rl.buckets[chatID] = b
	}
	return b
}

// wait blocks until a token is available for the given chat, then consumes one.
func (rl *rateLimiter) wait(chatID int64) {
	b := rl.getBucket(chatID)

	for {
		b.refill()
		if b.tokens >= 1 {
			b.tokens--
			return
		}
		// Sleep until at least one token is available
		deficit := 1.0 - b.tokens
		sleepDur := time.Duration(deficit/b.refillRate*1000) * time.Millisecond
		if sleepDur < 50*time.Millisecond {
			sleepDur = 50 * time.Millisecond
		}
		time.Sleep(sleepDur)
	}
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
	b.limiter.wait(msg.ChatID)

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
