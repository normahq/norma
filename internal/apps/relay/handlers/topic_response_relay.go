package handlers

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/tgbotkit/client"
)

type TopicResponseRelay struct {
	tgClient client.ClientWithResponsesInterface

	mu     sync.RWMutex
	topics map[int64]map[int]chan<- string
}

func NewTopicResponseRelay(tgClient client.ClientWithResponsesInterface) *TopicResponseRelay {
	return &TopicResponseRelay{
		tgClient: tgClient,
		topics:   make(map[int64]map[int]chan<- string),
	}
}

func (r *TopicResponseRelay) RegisterTopic(chatID int64, topicID int) <-chan string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.topics[chatID]; !exists {
		r.topics[chatID] = make(map[int]chan<- string)
	}

	ch := make(chan string, 100)
	r.topics[chatID][topicID] = ch

	go r.forwardResponses(chatID, topicID, ch)

	return ch
}

func (r *TopicResponseRelay) forwardResponses(chatID int64, topicID int, ch <-chan string) {
	for text := range ch {
		ctx := context.Background()
		req := client.SendMessageJSONRequestBody{
			ChatId:          chatID,
			Text:            text,
			MessageThreadId: &topicID,
		}

		resp, err := r.tgClient.SendMessageWithResponse(ctx, req)
		if err != nil {
			log.Error().Err(err).
				Int64("chat_id", chatID).
				Int("topic_id", topicID).
				Msg("Failed to send response to topic")
			continue
		}

		if resp.JSON200 == nil {
			log.Warn().
				Int64("chat_id", chatID).
				Int("topic_id", topicID).
				Str("status", resp.Status()).
				Msg("Failed to send response to topic")
		}
	}
}

func (r *TopicResponseRelay) UnregisterTopic(chatID int64, topicID int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if topicChans, exists := r.topics[chatID]; exists {
		if ch, ok := topicChans[topicID]; ok {
			close(ch)
			delete(topicChans, topicID)
		}
		if len(topicChans) == 0 {
			delete(r.topics, chatID)
		}
	}
}

func (r *TopicResponseRelay) SendToTopic(chatID int64, topicID int, text string) error {
	r.mu.RLock()
	topicChans, exists := r.topics[chatID]
	if !exists {
		r.mu.RUnlock()
		return fmt.Errorf("no registered topic for chat %d", chatID)
	}

	ch, ok := topicChans[topicID]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no registered topic %d for chat %d", topicID, chatID)
	}

	select {
	case ch <- text:
		return nil
	default:
		return fmt.Errorf("topic response channel full")
	}
}
