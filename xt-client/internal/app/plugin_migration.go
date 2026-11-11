package app

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"
	"xiaotao/xt-client/internal/sourceplugin"
	"xiaotao/xt-client/internal/storage"
)

// Legacy migration is opt-in and reads only the current environment's accounts.
func registerPluginMigration(mux *http.ServeMux, m *sourceplugin.Manager, data string, decrypt func([]byte) ([]byte, error), origin func(*http.Request) bool) {
	mux.HandleFunc("GET /api/source-plugins/legacy-trae", func(w http.ResponseWriter, r *http.Request) {
		if !origin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		accounts := []map[string]string{}
		store, err := storage.OpenReadOnly(filepath.Join(data, "accounts"))
		if err == nil {
			for _, a := range store.Accounts() {
				if a.Provider == "trae" && (a.Region == "cn" || a.Region == "") {
					accounts = append(accounts, map[string]string{"id": a.ID, "name": a.Nickname, "uid": a.UID})
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"accounts": accounts})
	})
	mux.HandleFunc("POST /api/source-plugins/legacy-trae", func(w http.ResponseWriter, r *http.Request) {
		if !origin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var input struct {
			ID string `json:"id"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil {
			http.Error(w, "invalid account", 400)
			return
		}
		store, err := storage.OpenReadOnly(filepath.Join(data, "accounts"))
		if err != nil {
			http.Error(w, "旧账号不可读", 400)
			return
		}
		account, ok := store.Account(input.ID)
		if !ok || account.Provider != "trae" || (account.Region != "cn" && account.Region != "") {
			http.Error(w, "旧账号不可迁移", 400)
			return
		}
		raw, err := store.LoadSecret(account.ID, decrypt)
		if err != nil {
			http.Error(w, "旧凭据不可解密", 400)
			return
		}
		var credential map[string]any
		if json.Unmarshal(raw, &credential) != nil {
			http.Error(w, "旧凭据格式无效", 400)
			return
		}
		credential["Region"] = "cn"
		credential["MachineID"] = account.MachineID
		credential["DeviceID"] = account.DeviceID
		raw, _ = json.Marshal(credential)
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		if _, err = m.Run(ctx, "xiaotao.trae", "credentials/import", raw, nil, "", nil); err != nil {
			http.Error(w, "迁移失败，请检查插件启用状态", 502)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
}
