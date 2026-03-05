package bot

import "sync"

// onboardingState tracks per-user onboarding progress.
type onboardingState struct {
	mu            sync.Mutex
	awaitingKey   map[int64]string // user ID -> backend they selected
	awaitingModel map[int64]bool   // user ID -> waiting for model name
}

func newOnboardingState() *onboardingState {
	return &onboardingState{
		awaitingKey:   make(map[int64]string),
		awaitingModel: make(map[int64]bool),
	}
}

func (o *onboardingState) setAwaitingKey(userID int64, backend string) {
	o.mu.Lock()
	o.awaitingKey[userID] = backend
	delete(o.awaitingModel, userID)
	o.mu.Unlock()
}

func (o *onboardingState) getAwaitingKey(userID int64) (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	b, ok := o.awaitingKey[userID]
	return b, ok
}

func (o *onboardingState) clearAwaitingKey(userID int64) {
	o.mu.Lock()
	delete(o.awaitingKey, userID)
	o.mu.Unlock()
}

func (o *onboardingState) setAwaitingModel(userID int64) {
	o.mu.Lock()
	o.awaitingModel[userID] = true
	delete(o.awaitingKey, userID)
	o.mu.Unlock()
}

func (o *onboardingState) getAwaitingModel(userID int64) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.awaitingModel[userID]
}

func (o *onboardingState) clearAwaitingModel(userID int64) {
	o.mu.Lock()
	delete(o.awaitingModel, userID)
	o.mu.Unlock()
}

// clear removes all onboarding state for a user.
func (o *onboardingState) clear(userID int64) {
	o.mu.Lock()
	delete(o.awaitingKey, userID)
	delete(o.awaitingModel, userID)
	o.mu.Unlock()
}
