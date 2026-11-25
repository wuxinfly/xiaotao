package office

import (
	"unicode/utf8"

	"xiaotao/xt-client/internal/agent"
)

const officeDraftLimit = 64 * 1024

type officeDraft struct {
	message agent.Message
	units   int
}

func (s *Service) observeLiveDraft(session string, event agent.Event) agent.Event {
	switch event.Type {
	case "message_delta":
		if event.Message == nil {
			return event
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		draft := s.liveDrafts[session]
		if draft.message.ID != event.Message.ID {
			draft = officeDraft{message: *event.Message}
			draft.message.Content = ""
		}
		offset := draft.units
		event.StreamOffset = &offset
		for _, r := range event.Message.Content {
			draft.units++
			if r > 0xffff {
				draft.units++
			}
		}
		remaining := officeDraftLimit - len(draft.message.Content)
		piece := event.Message.Content
		if remaining < utf8.UTFMax {
			piece = ""
		} else if len(piece) > remaining {
			for !utf8.RuneStart(piece[remaining]) {
				remaining--
			}
			piece = piece[:remaining]
		}
		draft.message.Content += piece
		if s.liveDrafts == nil {
			s.liveDrafts = make(map[string]officeDraft)
		}
		s.liveDrafts[session] = draft
	case "activity", "message", "answer_end":
		s.clearLiveDraft(session)
	}
	return event
}

func (s *Service) clearLiveDraft(session string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.liveDrafts, session)
}

func (s *Service) officeLiveHistory(session, before string) (*agent.HistoryPage, error) {
	page, err := s.exec.History(session, before)
	if err != nil || before != "" {
		return page, err
	}
	s.mu.Lock()
	draft := s.liveDrafts[session].message
	s.mu.Unlock()
	if draft.Content != "" {
		live := *page
		live.StreamDrafts = []agent.Message{draft}
		return &live, nil
	}
	return page, nil
}
