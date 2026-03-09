package email

import (
	"context"

	"github.com/rs/zerolog/log"
)

type ConsoleSender struct{}

func NewConsoleSender() *ConsoleSender {
	return &ConsoleSender{}
}

func (s *ConsoleSender) SendMagicLinkEmail(ctx context.Context, email string, token string) error {
	log.Info().Msgf("Sending magic link email to %s with token %s", email, token)
	return nil
}
