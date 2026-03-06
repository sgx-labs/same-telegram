package bot

import (
	"context"
	"sync"
	"time"
)

const onboardingTTL = 30 * time.Minute

// onboardingState tracks per-user onboarding progress.
type onboardingState struct {
	mu            sync.Mutex
	awaitingKey   map[int64]string    // user ID -> backend they selected
	awaitingModel map[int64]bool      // user ID -> waiting for model name
	timestamps    map[int64]time.Time // user ID -> last activity time
}

func newOnboardingState() *onboardingState {
	return &onboardingState{
		awaitingKey:   make(map[int64]string),
		awaitingModel: make(map[int64]bool),
		timestamps:    make(map[int64]time.Time),
	}
}

func (o *onboardingState) setAwaitingKey(userID int64, backend string) {
	o.mu.Lock()
	o.awaitingKey[userID] = backend
	delete(o.awaitingModel, userID)
	o.timestamps[userID] = time.Now()
	o.mu.Unlock()
}

func (o *onboardingState) getAwaitingKey(userID int64) (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	b, ok := o.awaitingKey[userID]
	if ok {
		if ts, has := o.timestamps[userID]; has && time.Now().After(ts.Add(onboardingTTL)) {
			delete(o.awaitingKey, userID)
			delete(o.timestamps, userID)
			return "", false
		}
	}
	return b, ok
}

func (o *onboardingState) clearAwaitingKey(userID int64) {
	o.mu.Lock()
	delete(o.awaitingKey, userID)
	if !o.awaitingModel[userID] {
		delete(o.timestamps, userID)
	}
	o.mu.Unlock()
}

func (o *onboardingState) setAwaitingModel(userID int64) {
	o.mu.Lock()
	o.awaitingModel[userID] = true
	delete(o.awaitingKey, userID)
	o.timestamps[userID] = time.Now()
	o.mu.Unlock()
}

func (o *onboardingState) getAwaitingModel(userID int64) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.awaitingModel[userID] {
		if ts, has := o.timestamps[userID]; has && time.Now().After(ts.Add(onboardingTTL)) {
			delete(o.awaitingModel, userID)
			delete(o.timestamps, userID)
			return false
		}
		return true
	}
	return false
}

func (o *onboardingState) clearAwaitingModel(userID int64) {
	o.mu.Lock()
	delete(o.awaitingModel, userID)
	if _, hasKey := o.awaitingKey[userID]; !hasKey {
		delete(o.timestamps, userID)
	}
	o.mu.Unlock()
}

// clear removes all onboarding state for a user.
func (o *onboardingState) clear(userID int64) {
	o.mu.Lock()
	delete(o.awaitingKey, userID)
	delete(o.awaitingModel, userID)
	delete(o.timestamps, userID)
	o.mu.Unlock()
}

// StartEviction runs a background goroutine that removes expired entries every 5 minutes.
func (o *onboardingState) StartEviction(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				o.evictExpired()
			}
		}
	}()
}

// evictExpired removes all expired onboarding entries.
func (o *onboardingState) evictExpired() {
	now := time.Now()
	o.mu.Lock()
	defer o.mu.Unlock()
	for uid, ts := range o.timestamps {
		if now.After(ts.Add(onboardingTTL)) {
			delete(o.awaitingKey, uid)
			delete(o.awaitingModel, uid)
			delete(o.timestamps, uid)
		}
	}
}
