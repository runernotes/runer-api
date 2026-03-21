package email

import (
	"context"

	"github.com/rs/zerolog/log"
)

type ConsoleSender struct{}

func NewConsoleSender() *ConsoleSender {
	return &ConsoleSender{}
}

func (s *ConsoleSender) SendMagicLinkEmail(ctx context.Context, email string, token string, isNewUser bool) error {
	if isNewUser {
		log.Info().Msgf("Sending magic link email to %s (new registration): runer://auth/new?token=%s", email, token)
	} else {
		log.Info().Msgf("Sending magic link email to %s (sign in): runer://auth/verify?token=%s", email, token)
	}
	return nil
}
