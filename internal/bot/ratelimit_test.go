package bot

import (
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestRateLimiterBurst(t *testing.T) {
	rl := newRateLimiter()

	// Should allow 3 messages immediately (burst)
	start := time.Now()
	for i := 0; i < 3; i++ {
		rl.wait(100)
	}
	elapsed := time.Since(start)

	// 3 burst messages should complete nearly instantly (< 100ms)
	if elapsed > 100*time.Millisecond {
		t.Errorf("3 burst messages took %v, expected < 100ms", elapsed)
	}
}

func TestRateLimiterThrottles(t *testing.T) {
	rl := newRateLimiter()

	// Consume burst
	for i := 0; i < 3; i++ {
		rl.wait(200)
	}

	// 4th message should require waiting
	start := time.Now()
	rl.wait(200)
	elapsed := time.Since(start)

	// Should have waited ~1 second (refill rate is 1/sec)
	if elapsed < 500*time.Millisecond {
		t.Errorf("4th message after burst took only %v, expected ~1s wait", elapsed)
	}
}

func TestRateLimiterPerChat(t *testing.T) {
	rl := newRateLimiter()

	// Consume burst for chat 300
	for i := 0; i < 3; i++ {
		rl.wait(300)
	}

	// Chat 400 should still have full burst available
	start := time.Now()
	for i := 0; i < 3; i++ {
		rl.wait(400)
	}
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("Different chat burst took %v, expected < 100ms", elapsed)
	}
}

func TestParseRetryAfter(t *testing.T) {
	// nil error
	if d := parseRetryAfter(nil); d != 0 {
		t.Errorf("nil error: got %v, want 0", d)
	}

	// Non-429 error
	if d := parseRetryAfter(&tgbotapi.Error{Code: 400, Message: "Bad Request"}); d != 0 {
		t.Errorf("400 error: got %v, want 0", d)
	}

	// 429 with RetryAfter
	err := &tgbotapi.Error{
		Code:    429,
		Message: "Too Many Requests",
		ResponseParameters: tgbotapi.ResponseParameters{
			RetryAfter: 5,
		},
	}
	if d := parseRetryAfter(err); d != 5*time.Second {
		t.Errorf("429 error: got %v, want 5s", d)
	}
}
