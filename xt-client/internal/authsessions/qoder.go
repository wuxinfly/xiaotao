package authsessions

import (
	"context"
	"errors"
	"time"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/qoder"
)

func (s *Service) startQoder(region string) (Session, error) {
	flow, err := qoder.StartDeviceFlow(region, "")
	if err != nil {
		return Session{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	now := time.Now()
	sess := &Session{ID: randHex(), Provider: "qoder", Region: region, Status: "waiting", CreatedAt: now.Unix(), ExpiresAt: now.Add(10 * time.Minute).Unix(), LoginURL: flow.AuthURL, cancel: cancel}
	s.sessions[sess.ID] = sess
	go func() {
		defer cancel()
		cred, err := (qoder.AuthClient{}).Wait(ctx, flow)
		s.finishQoder(sess, cred, err)
	}()
	return *sess, nil
}

func (s *Service) finishQoder(sess *Session, cred *providers.Credential, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess.Status != "waiting" {
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		sess.Status = "expired"
		return
	}
	if errors.Is(err, context.Canceled) {
		sess.Status = "cancelled"
		return
	}
	if err != nil {
		sess.Status = "failed"
		sess.Error = redact(err.Error())
		return
	}
	if cred == nil || cred.UID == "" || cred.AccessToken == "" {
		sess.Status = "failed"
		sess.Error = "Qoder login missing account identity"
		return
	}
	sess.Credential = cred
	sess.Status = "succeeded"
}

// Complete discards sensitive login state after the account has been saved.
func (s *Service) Complete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok && sess.Status == "succeeded" {
		sess.Credential = nil
		if sess.cancel != nil {
			sess.cancel()
		}
	}
}
