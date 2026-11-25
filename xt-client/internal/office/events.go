package office

import (
	"xiaotao/xt-client/internal/agent"
)

const taskEventBuffer = 64

func (s *Service) SubscribeTask(projectPath, taskID string) (<-chan agent.Event, func(), error) {
	if _, err := s.projectTask(projectPath, taskID); err != nil {
		return nil, nil, err
	}
	return s.subscribeEvents(taskID)
}

func (s *Service) SubscribeConversation(parent, path, member string) (<-chan agent.Event, func(), error) {
	scope, err := s.conversationScope(parent, path)
	if err != nil {
		return nil, nil, err
	}
	team, err := s.teams.Snapshot(scope.path)
	if err != nil {
		return nil, nil, err
	}
	if _, err := team.Member(member); err != nil {
		return nil, nil, ErrMemberNotFound
	}
	return s.subscribeEvents(conversationKey(parent, member))
}

func (s *Service) subscribeEvents(key string) (<-chan agent.Event, func(), error) {
	channel := make(chan agent.Event, taskEventBuffer)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, nil, ErrClosed
	}
	if s.subscribers == nil {
		s.subscribers = map[string]map[chan agent.Event]struct{}{}
	}
	if s.subscribers[key] == nil {
		s.subscribers[key] = map[chan agent.Event]struct{}{}
	}
	s.subscribers[key][channel] = struct{}{}
	s.mu.Unlock()
	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, exists := s.subscribers[key][channel]; exists {
			delete(s.subscribers[key], channel)
			close(channel)
			if len(s.subscribers[key]) == 0 {
				delete(s.subscribers, key)
			}
		}
	}
	return channel, unsubscribe, nil
}

func (s *Service) publishTask(taskID string, event agent.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for subscriber := range s.subscribers[taskID] {
		select {
		case subscriber <- event:
		default:
			// Events are a live view only. A slow reader must reload durable
			// history instead of blocking the agent or inventing progress.
		}
	}
}

func (s *Service) publishConversation(key string, event agent.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.RunStats != nil {
		stats := *event.RunStats
		event.RunStats = &stats
	}
	for subscriber := range s.subscribers[key] {
		select {
		case subscriber <- event:
		default:
			// Force a fresh history snapshot when a reader falls behind.
			delete(s.subscribers[key], subscriber)
			close(subscriber)
		}
	}
	if len(s.subscribers[key]) == 0 {
		delete(s.subscribers, key)
	}
}

// CloseEventSubscriptions releases long-lived HTTP readers during shutdown.
func (s *Service) CloseEventSubscriptions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for taskID, subscribers := range s.subscribers {
		for subscriber := range subscribers {
			close(subscriber)
		}
		delete(s.subscribers, taskID)
	}
}
