package telegram

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

type IPListAddRequest struct {
	AccountLabel string
	ListID       string
}

var ipListState = struct {
	mu        sync.Mutex
	pending   map[int64]IPListAddRequest
	callbacks map[string]IPListCallbackPayload
}{
	pending:   make(map[int64]IPListAddRequest),
	callbacks: make(map[string]IPListCallbackPayload),
}

func SetPendingIPListAdd(userID int64, req IPListAddRequest) {
	ipListState.mu.Lock()
	defer ipListState.mu.Unlock()
	ipListState.pending[userID] = req
}

func GetPendingIPListAdd(userID int64) (IPListAddRequest, bool) {
	ipListState.mu.Lock()
	defer ipListState.mu.Unlock()
	req, ok := ipListState.pending[userID]
	return req, ok
}

func ClearPendingIPListAdd(userID int64) {
	ipListState.mu.Lock()
	defer ipListState.mu.Unlock()
	delete(ipListState.pending, userID)
}

type IPListCallbackPayload struct {
	AccountLabel string
	ListID       string
	ItemID       string
}

func SetIPListCallbackPayload(payload IPListCallbackPayload) string {
	token := newIPListToken()
	ipListState.mu.Lock()
	defer ipListState.mu.Unlock()
	ipListState.callbacks[token] = payload
	return token
}

func GetIPListCallbackPayload(token string) (IPListCallbackPayload, bool) {
	ipListState.mu.Lock()
	defer ipListState.mu.Unlock()
	payload, ok := ipListState.callbacks[token]
	return payload, ok
}

func newIPListToken() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte("fallback"))
	}
	return hex.EncodeToString(buf)
}
