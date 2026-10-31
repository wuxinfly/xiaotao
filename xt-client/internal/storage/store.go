package storage

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const secretPrefix = "b64:"

type QuotaItem struct {
	Label    string   `json:"label"`
	Used     *float64 `json:"used,omitempty"`
	Total    *float64 `json:"total,omitempty"`
	Unit     string   `json:"unit"`
	ExpireAt int64    `json:"expireAt,omitempty"`
}

type QuotaSnapshot struct {
	Status    string      `json:"status"`
	Items     []QuotaItem `json:"items"`
	Error     string      `json:"error,omitempty"`
	FetchedAt int64       `json:"fetchedAt"`
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
	UsageTokens int64          `json:"usageTokens"`
	UsageCalls  int64          `json:"usageCalls"`
}

type ProxySettings struct {
	DefaultAccountID string `json:"defaultAccountId"`
}

type fileData struct {
	Version  int               `json:"version"`
	Accounts []*Account        `json:"accounts"`
	Proxy    ProxySettings     `json:"proxy"`
	Secrets  map[string]string `json:"secrets"`
}

type Store struct {
	path string
	mu   sync.Mutex
	data fileData
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
	sort.Slice(out, func(i, j int) bool { return out[i].AddedAt < out[j].AddedAt })
	return out
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
	if s.data.Proxy.DefaultAccountID == id {
		s.data.Proxy.DefaultAccountID = ""
	}
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

func (s *Store) Proxy() ProxySettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.data.Proxy
	if p.DefaultAccountID == "" {
		for _, a := range s.data.Accounts {
			if a.Enabled {
				p.DefaultAccountID = a.ID
				break
			}
		}
	}
	return p
}

func (s *Store) SetProxy(p ProxySettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Proxy = p
	return s.save()
}
