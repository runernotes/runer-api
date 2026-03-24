package email

import (
	"context"
	"fmt"

	resend "github.com/resend/resend-go/v2"
)

// ResendSender sends magic link emails via the Resend API.
type ResendSender struct {
	emails   resend.EmailsSvc
	fromAddr string
}

// NewResendSender creates a ResendSender using the given API key and from address.
func NewResendSender(apiKey, fromAddr string) *ResendSender {
	client := resend.NewClient(apiKey)
	return &ResendSender{emails: client.Emails, fromAddr: fromAddr}
}

func (s *ResendSender) SendMagicLinkEmail(ctx context.Context, email string, token string, isNewUser bool) error {
	subject, deepLink := magicLinkContent(isNewUser, token)

	html := fmt.Sprintf(`<p>Click the link below to sign in to Runer:</p>
<p><a href="%s">%s</a></p>
<p>This link expires in 1 hour. If you did not request this, you can safely ignore this email.</p>`,
		deepLink, deepLink)

	params := &resend.SendEmailRequest{
		From:    s.fromAddr,
		To:      []string{email},
		Subject: subject,
		Html:    html,
	}

	_, err := s.emails.SendWithContext(ctx, params)
	return err
}

func magicLinkContent(isNewUser bool, token string) (subject, deepLink string) {
	if isNewUser {
		return "Welcome to Runer — confirm your email", fmt.Sprintf("runer://auth/new?token=%s", token)
	}
	return "Your Runer sign-in link", fmt.Sprintf("runer://auth/verify?token=%s", token)
}
