package storage

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const secretPrefix = "b64:"

type QuotaItem struct {
	Label     string   `json:"label"`
	Used      *float64 `json:"used,omitempty"`
	Total     *float64 `json:"total,omitempty"`
	Remaining *float64 `json:"remaining,omitempty"`
	Unit      string   `json:"unit"`
	ExpireAt  int64    `json:"expireAt,omitempty"`
}

type QuotaSnapshot struct {
	Status    string      `json:"status"`
	Items     []QuotaItem `json:"items"`
	Error     string      `json:"error,omitempty"`
	FetchedAt int64       `json:"fetchedAt"`
}

type CheckinStatus struct {
	Supported   bool    `json:"supported"`
	Active      bool    `json:"active"`
	CheckedIn   bool    `json:"checkedIn"`
	StreakDays  int     `json:"streakDays"`
	DailyCredit float64 `json:"dailyCredit"`
	Message     string  `json:"message"`
	FetchedAt   int64   `json:"fetchedAt"`
}

type Account struct {
	ID          string         `json:"id"`
	Provider    string         `json:"provider"`
	Region      string         `json:"region"`
	UID         string         `json:"uid"`
	Nickname    string         `json:"nickname"`
	Alias       string         `json:"alias"`
	Enabled     bool           `json:"enabled"`
	AuthState   string         `json:"authState"`
	MachineID   string         `json:"machineId"`
	DeviceID    string         `json:"deviceId"`
	AddedAt     int64          `json:"addedAt"`
	UpdatedAt   int64          `json:"updatedAt"`
	Quota       *QuotaSnapshot `json:"quota,omitempty"`
	Checkin     *CheckinStatus `json:"checkin,omitempty"`
	UsageTokens int64          `json:"usageTokens"`
	UsageCalls  int64          `json:"usageCalls"`
}

type fileData struct {
	Version  int               `json:"version"`
	Accounts []*Account        `json:"accounts"`
	Secrets  map[string]string `json:"secrets"`
}

type Store struct {
	path string
	mu   sync.Mutex
	data fileData
}

// Export returns account metadata and decrypted account credentials for a
// one-time environment bootstrap. Internal secrets such as the proxy key are
// excluded. The source store is never modified.
func (s *Store) Export(dec func([]byte) ([]byte, error)) ([]*Account, map[string][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	accounts := make([]*Account, 0, len(s.data.Accounts))
	secrets := make(map[string][]byte, len(s.data.Accounts))
	for _, acct := range s.data.Accounts {
		if acct == nil {
			continue
		}
		value, ok := s.data.Secrets[acct.ID]
		if !ok {
			continue
		}
		plain, err := decodeSecret(value, acct.ID, dec)
		if err != nil {
			continue
		}
		copy := *acct
		if acct.Quota != nil {
			quota := *acct.Quota
			copy.Quota = &quota
		}
		accounts = append(accounts, &copy)
		secrets[acct.ID] = plain
	}
	return accounts, secrets
}

func (s *Store) Bootstrap(accounts []*Account, secrets map[string][]byte, enc func([]byte) ([]byte, error)) error {
	encoded := make(map[string]string, len(secrets))
	for id, plain := range secrets {
		blob, err := enc(plain)
		if err != nil {
			return err
		}
		encoded[id] = secretPrefix + base64.StdEncoding.EncodeToString(blob)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.data.Accounts) > 0 || len(s.data.Secrets) > 0 {
		return nil
	}
	previousAccounts := s.data.Accounts
	previousSecrets := s.data.Secrets
	s.data.Accounts = accounts
	s.data.Secrets = encoded
	if err := s.save(); err != nil {
		s.data.Accounts = previousAccounts
		s.data.Secrets = previousSecrets
		return err
	}
	return nil
}

func (s *Store) SecretRevision(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.data.Secrets[id]
	if !ok {
		return "", fmt.Errorf("account credential unavailable")
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value))), nil
}

// ReplaceSecret rejects refresh results after deletion or a new login.
func (s *Store) ReplaceSecret(id, revision string, plain []byte, enc func([]byte) ([]byte, error)) error {
	blob, err := enc(plain)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for _, acct := range s.data.Accounts {
		if acct.ID == id {
			found = true
			break
		}
	}
	current, ok := s.data.Secrets[id]
	if !found || !ok || fmt.Sprintf("%x", sha256.Sum256([]byte(current))) != revision {
		return fmt.Errorf("account credential changed during refresh")
	}
	s.data.Secrets[id] = secretPrefix + base64.StdEncoding.EncodeToString(blob)
	if err := s.save(); err != nil {
		s.data.Secrets[id] = current
		return err
	}
	return nil
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "accounts.json")
	s := &Store{path: path, data: fileData{Version: 1, Secrets: map[string]string{}}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, s.save()
	}
	if err != nil {
		return nil, err
	}
	return load(s, b)
}

// OpenReadOnly loads an existing store without creating or modifying files.
func OpenReadOnly(dir string) (*Store, error) {
	path := filepath.Join(dir, "accounts.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := &Store{path: path}
	return load(s, b)
}

func load(s *Store, b []byte) (*Store, error) {
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("invalid accounts data: %w", err)
	}
	if s.data.Version != 1 {
		return nil, fmt.Errorf("unsupported accounts data version %d", s.data.Version)
	}
	if s.data.Secrets == nil {
		s.data.Secrets = map[string]string{}
	}
	if s.data.Accounts == nil {
		s.data.Accounts = []*Account{}
	}
	return s, nil
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".accounts-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}

func (s *Store) Accounts() []*Account {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Account, 0, len(s.data.Accounts))
	for _, a := range s.data.Accounts {
		cp := *a
		if a.Quota != nil {
			q := *a.Quota
			cp.Quota = &q
		}
		out = append(out, &cp)
	}
	return out
}

// ReorderAccounts persists the request order for one provider while keeping
// every account in the same global slots. Account routing reads this order.
func (s *Store) ReorderAccounts(providerID string, orderedIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	providerID = strings.ToLower(providerID)
	if providerID == "" {
		return fmt.Errorf("provider is required")
	}
	current := make([]*Account, 0)
	byID := make(map[string]*Account)
	for _, acct := range s.data.Accounts {
		if acct == nil || !strings.EqualFold(acct.Provider, providerID) {
			continue
		}
		current = append(current, acct)
		byID[acct.ID] = acct
	}
	if len(orderedIDs) != len(current) {
		return fmt.Errorf("account order must include every account in %s", providerID)
	}

	ordered := make([]*Account, 0, len(orderedIDs))
	seen := make(map[string]struct{}, len(orderedIDs))
	for _, id := range orderedIDs {
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate account %q in order", id)
		}
		acct, ok := byID[id]
		if !ok {
			return fmt.Errorf("account %q is not in %s", id, providerID)
		}
		seen[id] = struct{}{}
		ordered = append(ordered, acct)
	}

	previous := s.data.Accounts
	next := make([]*Account, len(previous))
	position := 0
	for i, acct := range previous {
		if acct != nil && strings.EqualFold(acct.Provider, providerID) {
			next[i] = ordered[position]
			position++
			continue
		}
		next[i] = acct
	}
	s.data.Accounts = next
	if err := s.save(); err != nil {
		s.data.Accounts = previous
		return err
	}
	return nil
}

func (s *Store) Account(id string) (Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.data.Accounts {
		if a.ID == id {
			cp := *a
			if a.Quota != nil {
				q := *a.Quota
				cp.Quota = &q
			}
			return cp, true
		}
	}
	return Account{}, false
}

func (s *Store) UpsertAccount(a *Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a.UpdatedAt = time.Now().Unix()
	replaced := false
	for i, cur := range s.data.Accounts {
		if cur.ID == a.ID {
			alias := cur.Alias
			if a.Alias == "" {
				a.Alias = alias
			}
			usage, calls := cur.UsageTokens, cur.UsageCalls
			quota := cur.Quota
			a.Alias = alias
			a.UsageTokens, a.UsageCalls = usage, calls
			a.Quota = quota
			s.data.Accounts[i] = a
			replaced = true
			break
		}
	}
	if !replaced {
		s.data.Accounts = append(s.data.Accounts, a)
	}
	return s.save()
}

func (s *Store) UpdateAccount(id string, fn func(*Account)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.data.Accounts {
		if a.ID == id {
			fn(a)
			a.UpdatedAt = time.Now().Unix()
			return s.save()
		}
	}
	return fmt.Errorf("account %q not found", id)
}

func (s *Store) DeleteAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.data.Accounts[:0]
	for _, a := range s.data.Accounts {
		if a.ID != id {
			kept = append(kept, a)
		}
	}
	s.data.Accounts = kept
	delete(s.data.Secrets, id)
	return s.save()
}

func (s *Store) SaveSecret(id string, plain []byte, enc func([]byte) ([]byte, error)) error {
	blob, err := enc(plain)
	if err != nil {
		return err
	}
	// Encrypted blobs are binary; encode them so they survive the JSON
	// round-trip (Go replaces invalid UTF-8 bytes with U+FFFD).
	stored := secretPrefix + base64.StdEncoding.EncodeToString(blob)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Secrets[id] = stored
	return s.save()
}

func (s *Store) LoadSecret(id string, dec func([]byte) ([]byte, error)) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data.Secrets[id]
	if !ok {
		return nil, fmt.Errorf("secret for %q not found", id)
	}
	if rest, ok := strings.CutPrefix(v, secretPrefix); ok {
		raw, err := base64.StdEncoding.DecodeString(rest)
		if err != nil {
			return nil, fmt.Errorf("secret for %q is malformed", id)
		}
		return dec(raw)
	}
	// Legacy entries stored raw binary as JSON strings are corrupted and
	// will fail decryption; surface the decoder error.
	return dec([]byte(v))
}

func decodeSecret(value, id string, dec func([]byte) ([]byte, error)) ([]byte, error) {
	if rest, ok := strings.CutPrefix(value, secretPrefix); ok {
		raw, err := base64.StdEncoding.DecodeString(rest)
		if err != nil {
			return nil, fmt.Errorf("secret for %q is malformed", id)
		}
		return dec(raw)
	}
	return dec([]byte(value))
}
