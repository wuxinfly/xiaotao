package sourceplugin

import (
	"bytes"
	"errors"
	"slices"
)

type PluginAccount struct {
	ID       string `json:"id"`
	Identity string `json:"identity,omitempty"`
	Name     string `json:"name"`
	Secret   []byte `json:"secret,omitempty"`
}

func publicAccounts(accounts []PluginAccount) []PluginAccount {
	out := slices.Clone(accounts)
	for i := range out {
		out[i].Secret = nil
	}
	return out
}

func (s Saved) accountFor(id string, adding bool) (PluginAccount, error) {
	if adding || (id == "" && len(s.Accounts) == 0) {
		return PluginAccount{}, nil
	}
	for _, account := range s.Accounts {
		if id == "" || account.ID == id {
			return account, nil
		}
	}
	return PluginAccount{}, errors.New("插件账号不存在")
}

func (m *Manager) PrimaryAccount(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, saved := range m.entries {
		if saved.ID == id && len(saved.Accounts) > 0 {
			return saved.Accounts[0].ID
		}
	}
	return ""
}

func (m *Manager) applyAccountResult(next *Saved, previous PluginAccount, result Result, credential []byte, adding bool) (bool, error) {
	if len(result.Credential) == 0 {
		return false, nil
	}
	account := previous
	if adding && result.Account != nil && result.Account.ID != "" {
		account.ID = route(next.ID, "account:"+result.Account.ID)
		for _, entry := range next.Accounts {
			if entry.Identity == result.Account.ID {
				account = entry
				break
			}
		}
	}
	if account.ID == "" {
		account.ID = "original"
	}
	if result.Account != nil && result.Account.ID != "" {
		account.Identity = result.Account.ID
		if result.Account.Name != "" {
			account.Name = result.Account.Name
		}
	}
	if account.Name == "" {
		account.Name = "已导入账号"
	}
	if !adding && account.ID == previous.ID && account.Identity == previous.Identity && account.Name == previous.Name && bytes.Equal(credential, result.Credential) {
		return false, nil
	}
	secret, err := m.encrypt(result.Credential)
	if err != nil {
		return false, err
	}
	account.Secret = secret
	next.Accounts = slices.Clone(next.Accounts)
	for i, entry := range next.Accounts {
		if entry.ID == account.ID {
			next.Accounts[i] = account
			return true, nil
		}
	}
	next.Accounts = append(next.Accounts, account)
	return true, nil
}

func (m *Manager) ReorderAccounts(id string, order []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, entry := range m.entries {
		if entry.ID != id {
			continue
		}
		if len(order) != len(entry.Accounts) {
			break
		}
		byID := make(map[string]PluginAccount, len(entry.Accounts))
		for _, account := range entry.Accounts {
			byID[account.ID] = account
		}
		sorted := make([]PluginAccount, 0, len(order))
		for _, key := range order {
			account, ok := byID[key]
			if !ok {
				return errors.New("账号列表已变化，请刷新后重试")
			}
			sorted = append(sorted, account)
			delete(byID, key)
		}
		m.entries[i].Accounts = sorted
		if err := m.persist(); err != nil {
			m.entries[i] = entry
			return errors.New("无法保存账号顺序")
		}
		return nil
	}
	return errors.New("账号列表已变化，请刷新后重试")
}

func (m *Manager) Supports(id, capability string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range m.entries {
		if entry.ID == id {
			return slices.Contains(entry.Manifest.Capabilities, capability)
		}
	}
	return false
}
