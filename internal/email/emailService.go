package email

import "context"

type EmailService struct {
	emailSender emailSender
}

type emailSender interface {
	SendMagicLinkEmail(ctx context.Context, email string, token string) error
}

func NewEmailService(emailSender emailSender) *EmailService {
	return &EmailService{emailSender: emailSender}
}

func (s *EmailService) SendMagicLinkEmail(ctx context.Context, email string, token string) error {
	return s.emailSender.SendMagicLinkEmail(ctx, email, token)
}
