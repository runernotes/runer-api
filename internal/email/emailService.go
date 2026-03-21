package email

import "context"

type EmailService struct {
	emailSender emailSender
}

// Sender is the interface for sending emails. Implement it to inject a custom sender (e.g. in tests).
type Sender interface {
	SendMagicLinkEmail(ctx context.Context, email string, token string, isNewUser bool) error
}

// emailSender is an alias kept for internal use.
type emailSender = Sender

func NewEmailService(emailSender Sender) *EmailService {
	return &EmailService{emailSender: emailSender}
}

func (s *EmailService) SendMagicLinkEmail(ctx context.Context, email string, token string, isNewUser bool) error {
	return s.emailSender.SendMagicLinkEmail(ctx, email, token, isNewUser)
}
