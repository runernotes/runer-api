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
	// baseURL is the public API base URL (e.g. https://api.runer.app) used to
	// build the HTTPS redirect link embedded in emails. Email clients such as
	// Gmail block custom URI schemes (runer://) but always render https:// links
	// as clickable; the redirect endpoint then bounces the user to the deep link.
	baseURL string
}

// NewResendSender creates a ResendSender using the given API key, from address,
// and public API base URL.
func NewResendSender(apiKey, fromAddr, baseURL string) *ResendSender {
	client := resend.NewClient(apiKey)
	return &ResendSender{emails: client.Emails, fromAddr: fromAddr, baseURL: baseURL}
}

func (s *ResendSender) SendMagicLinkEmail(ctx context.Context, email string, token string, isNewUser bool) error {
	subject, redirectURL := magicLinkContent(isNewUser, token, s.baseURL)

	html := fmt.Sprintf(`<p>Click the link below to sign in to Runer:</p>
<p><a href="%s">Sign in to Runer</a></p>
<p>This link expires in 15 minutes. If you did not request this, you can safely ignore this email.</p>`,
		redirectURL)

	params := &resend.SendEmailRequest{
		From:    s.fromAddr,
		To:      []string{email},
		Subject: subject,
		Html:    html,
	}

	_, err := s.emails.SendWithContext(ctx, params)
	return err
}

// magicLinkContent builds the email subject and the HTTPS redirect URL for the
// magic link. Both new and returning users receive the same redirect URL — the
// API endpoint bounces them to runer://auth/verify, which the app handles.
// Using an HTTPS URL ensures the link is rendered as clickable in all email
// clients (Gmail and others block custom-scheme hrefs such as runer://).
func magicLinkContent(isNewUser bool, token, baseURL string) (subject, redirectURL string) {
	link := baseURL + "/api/v1/auth/verify-redirect?token=" + token
	if isNewUser {
		return "Welcome to Runer — confirm your email", link
	}
	return "Your Runer sign-in link", link
}
