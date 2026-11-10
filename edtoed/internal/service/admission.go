package service

import "sync"

// OrganizationLimiter bounds admitted analyses per organization on one instance.
type OrganizationLimiter struct {
	max    int
	mu     sync.Mutex
	active map[string]int
}

func NewOrganizationLimiter(max int) *OrganizationLimiter {
	return &OrganizationLimiter{max: max, active: make(map[string]int)}
}

func (l *OrganizationLimiter) acquire(org string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active[org] >= l.max {
		return false
	}
	l.active[org]++
	return true
}

func (l *OrganizationLimiter) release(org string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active[org] == 1 {
		delete(l.active, org)
		return
	}
	l.active[org]--
}
