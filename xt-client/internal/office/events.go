package office

import (
	"xiaotao/xt-client/internal/agent"
)

const taskEventBuffer = 64

func (s *Service) SubscribeTask(projectPath, taskID string) (<-chan agent.Event, func(), error) {
	if _, err := s.projectTask(projectPath, taskID); err != nil {
		return nil, nil, err
	}
	channel := make(chan agent.Event, taskEventBuffer)
	s.mu.Lock()
	if s.subscribers == nil {
		s.subscribers = map[string]map[chan agent.Event]struct{}{}
	}
	if s.subscribers[taskID] == nil {
		s.subscribers[taskID] = map[chan agent.Event]struct{}{}
	}
	s.subscribers[taskID][channel] = struct{}{}
	s.mu.Unlock()
	var unsubscribed bool
	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if unsubscribed {
			return
		}
		unsubscribed = true
		delete(s.subscribers[taskID], channel)
		close(channel)
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

func (s *Service) closeTaskEvents() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for taskID, subscribers := range s.subscribers {
		for subscriber := range subscribers {
			close(subscriber)
		}
		delete(s.subscribers, taskID)
	}
}
