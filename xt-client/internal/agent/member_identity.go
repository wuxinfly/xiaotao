package agent

import "fmt"

func (s *Service) SetMember(sessionID string, member *MemberIdentity) error {
	if member == nil || member.ID == "" || member.Name == "" {
		return fmt.Errorf("成员身份无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	history, err := s.loadSession(sessionID)
	if err != nil {
		return err
	}
	copied := *member
	history.Member = &copied
	return s.saveSession(history)
}
