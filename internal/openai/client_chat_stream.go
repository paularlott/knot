package openai

import "context"

type ChatStream struct {
	responseChan <-chan ChatCompletionResponse
	errorChan    <-chan error
	ctx          context.Context
	current      *ChatCompletionResponse
	err          error
	done         bool
}

func (s *ChatStream) Next() bool {
	if s.done {
		return false
	}

	select {
	case response, ok := <-s.responseChan:
		if !ok {
			s.done = true
			return false
		}
		s.current = &response
		return true

	case err := <-s.errorChan:
		s.err = err
		s.done = true
		return false

	case <-s.ctx.Done():
		s.err = s.ctx.Err()
		s.done = true
		return false
	}
}

func (s *ChatStream) Current() ChatCompletionResponse {
	if s.current == nil {
		return ChatCompletionResponse{}
	}
	return *s.current
}

func (s *ChatStream) Err() error {
	return s.err
}
