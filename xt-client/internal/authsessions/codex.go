package authsessions

import (
	"context"
	"fmt"
	"sync"
	"time"

	"xiaotao/xt-client/internal/logging"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/codex"
)

var (
	codexFlowsMu sync.Mutex
	codexFlows   = map[string]*codex.LoginFlow{}
)

func (s *Service) startCodex() (Session, error) {
	flow, err := codex.StartLoginFlow()
	if err != nil {
		return Session{}, err
	}
	now := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	sess := &Session{
		ID:        flow.ID,
		Provider:  "codex",
		Region:    "global",
		Status:    "waiting",
		CreatedAt: now.Unix(),
		ExpiresAt: now.Add(15 * time.Minute).Unix(),
		LoginURL:  flow.LoginURL,
		cancel: func() {
			flow.Cancel()
			cancel()
		},
	}
	s.sessions[sess.ID] = sess
	logging.Default().For(logging.ModuleAuth).Info("login started",
		logging.F("provider", "codex"),
		logging.F("region", "global"),
		logging.F("session", sess.ID),
	)

	codexFlowsMu.Lock()
	codexFlows[sess.ID] = flow
	codexFlowsMu.Unlock()

	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if flow.IsDone() {
				cred, err := flow.Result()
				s.finishCodex(sess, cred, err)
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()

	return *sess, nil
}

func (s *Service) CompleteCodexManual(sessionID, rawCallback string) (*providers.Credential, error) {
	codexFlowsMu.Lock()
	flow, ok := codexFlows[sessionID]
	codexFlowsMu.Unlock()
	if !ok {
		logging.Default().For(logging.ModuleAuth).Warn("manual callback error",
			logging.F("provider", "codex"),
			logging.F("session", sessionID),
			logging.F("error", "session not found"),
		)
		return nil, fmt.Errorf("session not found")
	}
	cred, err := flow.CompleteManual(rawCallback)
	if err != nil {
		logging.Default().For(logging.ModuleAuth).Warn("manual callback error",
			logging.F("provider", "codex"),
			logging.F("session", sessionID),
			logging.F("error", logging.Redact(err.Error(), 200)),
		)
		return nil, err
	}
	s.mu.Lock()
	if sess, ok := s.sessions[sessionID]; ok {
		sess.Status = "succeeded"
		sess.Credential = cred
	}
	s.mu.Unlock()
	logging.Default().For(logging.ModuleAuth).Info("manual callback completed",
		logging.F("provider", "codex"),
		logging.F("session", sessionID),
	)
	return cred, nil
}

func (s *Service) finishCodex(sess *Session, cred *providers.Credential, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess.Status != "waiting" {
		return
	}
	durMs := (time.Now().Unix() - sess.CreatedAt) * 1000
	if err != nil {
		sess.Status = "failed"
		sess.Error = redact(err.Error())
		logging.Default().For(logging.ModuleAuth).Warn("login failed",
			logging.F("provider", "codex"),
			logging.F("session", sess.ID),
			logging.F("duration_ms", durMs),
			logging.F("error", logging.Redact(err.Error(), 200)),
		)
		return
	}
	if cred == nil || cred.AccessToken == "" {
		sess.Status = "failed"
		sess.Error = "Codex login missing credential"
		logging.Default().For(logging.ModuleAuth).Warn("login failed",
			logging.F("provider", "codex"),
			logging.F("session", sess.ID),
			logging.F("duration_ms", durMs),
			logging.F("error", "Codex login missing credential"),
		)
		return
	}
	sess.Credential = cred
	sess.Status = "succeeded"
	logging.Default().For(logging.ModuleAuth).Info("login succeeded",
		logging.F("provider", "codex"),
		logging.F("session", sess.ID),
		logging.F("duration_ms", durMs),
	)
}
