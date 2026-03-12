package bot

import (
	"context"
	"sync"
	"time"
)

const onboardingTTL = 30 * time.Minute

// onboardingState tracks per-user onboarding progress.
type onboardingState struct {
	mu             sync.Mutex
	awaitingKey    map[int64]string                       // user ID -> backend they selected
	awaitingModel  map[int64]bool                         // user ID -> waiting for model name
	workspaces     map[int64]*workspaceOnboardingState     // user ID -> workspace onboarding
	newbotFlows    map[int64]*newbotState                 // user ID -> /newbot flow state
	timestamps     map[int64]time.Time                    // user ID -> last activity time
	invitedUsers   map[int64]bool                         // user IDs that have used an invite code
	pendingDestroy map[int64]time.Time                    // user ID -> when /destroy was requested (expires after 60s)
	pendingImport  map[int64]time.Time                    // user ID -> when /import was requested (expires after 5 min)
}

func newOnboardingState() *onboardingState {
	return &onboardingState{
		awaitingKey:    make(map[int64]string),
		awaitingModel:  make(map[int64]bool),
		workspaces:     make(map[int64]*workspaceOnboardingState),
		newbotFlows:    make(map[int64]*newbotState),
		timestamps:     make(map[int64]time.Time),
		invitedUsers:   make(map[int64]bool),
		pendingDestroy: make(map[int64]time.Time),
		pendingImport:  make(map[int64]time.Time),
	}
}

// markInvited records that a user used an invite code.
func (o *onboardingState) markInvited(userID int64) {
	o.mu.Lock()
	o.invitedUsers[userID] = true
	o.mu.Unlock()
}

// isInvited returns true if the user has already used an invite code.
func (o *onboardingState) isInvited(userID int64) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.invitedUsers[userID]
}

// inviteCount returns the number of users who have used invite codes.
func (o *onboardingState) inviteCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.invitedUsers)
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

// setPendingImport marks a user as waiting for a file upload to import.
func (o *onboardingState) setPendingImport(userID int64) {
	o.mu.Lock()
	o.pendingImport[userID] = time.Now()
	o.mu.Unlock()
}

// hasPendingImport returns true if the user has a pending import that hasn't expired (5 min TTL).
func (o *onboardingState) hasPendingImport(userID int64) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	ts, ok := o.pendingImport[userID]
	if !ok {
		return false
	}
	if time.Since(ts) > 5*time.Minute {
		delete(o.pendingImport, userID)
		return false
	}
	return true
}

// clearPendingImport removes the pending import state for a user.
func (o *onboardingState) clearPendingImport(userID int64) {
	o.mu.Lock()
	delete(o.pendingImport, userID)
	o.mu.Unlock()
}

// clear removes all onboarding state for a user.
func (o *onboardingState) clear(userID int64) {
	o.mu.Lock()
	delete(o.awaitingKey, userID)
	delete(o.awaitingModel, userID)
	delete(o.newbotFlows, userID)
	delete(o.timestamps, userID)
	delete(o.pendingImport, userID)
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
	// Evict expired pending imports (5 min TTL).
	for uid, ts := range o.pendingImport {
		if now.After(ts.Add(5 * time.Minute)) {
			delete(o.pendingImport, uid)
		}
	}
}
