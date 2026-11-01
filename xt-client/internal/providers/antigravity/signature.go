package antigravity

import (
	"container/list"
	"strings"
	"sync"
)

const sentinelSignature = "skip_thought_signature_validator"
const maxSignatureEntries = 2048

type signatureEntry struct {
	callID    string
	signature string
	name      string
}

// SignatureCache is a bounded, per-account in-memory cache for Gemini
// thoughtSignature round-trips.  A fresh cache is created for each account,
// so signatures never cross accounts.
type SignatureCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List
}

func NewSignatureCache() *SignatureCache {
	return &SignatureCache{entries: map[string]*list.Element{}, order: list.New()}
}

func (c *SignatureCache) Remember(callID, signature, name string) {
	if callID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[callID]; ok {
		entry := elem.Value.(*signatureEntry)
		entry.signature = signature
		entry.name = name
		c.order.MoveToBack(elem)
		return
	}
	entry := &signatureEntry{callID: callID, signature: signature, name: name}
	elem := c.order.PushBack(entry)
	c.entries[callID] = elem
	for c.order.Len() > maxSignatureEntries {
		oldest := c.order.Front()
		if oldest == nil {
			break
		}
		e := oldest.Value.(*signatureEntry)
		delete(c.entries, e.callID)
		c.order.Remove(oldest)
	}
}

func (c *SignatureCache) Lookup(callID string) (string, string) {
	if callID == "" {
		return "", ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[callID]
	if !ok {
		return "", ""
	}
	entry := elem.Value.(*signatureEntry)
	c.order.MoveToBack(elem)
	return entry.signature, entry.name
}

// Resolve mirrors buddy-proxy: an explicit or cached signature wins; Gemini
// models accept the documented sentinel when the signature is unavailable.
func (c *SignatureCache) Resolve(callID, model string) string {
	signature, _ := c.Lookup(callID)
	if signature != "" {
		return signature
	}
	if strings.Contains(strings.ToLower(model), "gemini") {
		return sentinelSignature
	}
	return ""
}
