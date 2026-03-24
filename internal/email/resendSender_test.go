package email

import (
	"context"
	"testing"

	resend "github.com/resend/resend-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubEmailsSvc captures the last SendEmailRequest for assertions.
type stubEmailsSvc struct {
	lastReq *resend.SendEmailRequest
	err     error
}

func (s *stubEmailsSvc) SendWithContext(_ context.Context, params *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
	s.lastReq = params
	return &resend.SendEmailResponse{Id: "stub-id"}, s.err
}

// Unused interface methods — satisfy resend.EmailsSvc.
func (s *stubEmailsSvc) Send(params *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
	return s.SendWithContext(context.Background(), params)
}
func (s *stubEmailsSvc) SendWithOptions(_ context.Context, params *resend.SendEmailRequest, _ *resend.SendEmailOptions) (*resend.SendEmailResponse, error) {
	return s.SendWithContext(context.Background(), params)
}
func (s *stubEmailsSvc) Get(_ string) (*resend.Email, error)                                         { return nil, nil }
func (s *stubEmailsSvc) GetWithContext(_ context.Context, _ string) (*resend.Email, error)           { return nil, nil }
func (s *stubEmailsSvc) Cancel(_ string) (*resend.CancelScheduledEmailResponse, error)               { return nil, nil }
func (s *stubEmailsSvc) CancelWithContext(_ context.Context, _ string) (*resend.CancelScheduledEmailResponse, error) {
	return nil, nil
}
func (s *stubEmailsSvc) Update(_ *resend.UpdateEmailRequest) (*resend.UpdateEmailResponse, error) { return nil, nil }
func (s *stubEmailsSvc) UpdateWithContext(_ context.Context, _ *resend.UpdateEmailRequest) (*resend.UpdateEmailResponse, error) {
	return nil, nil
}
func (s *stubEmailsSvc) List() (resend.ListEmailsResponse, error) { return resend.ListEmailsResponse{}, nil }
func (s *stubEmailsSvc) ListWithContext(_ context.Context) (resend.ListEmailsResponse, error) {
	return resend.ListEmailsResponse{}, nil
}
func (s *stubEmailsSvc) ListWithOptions(_ context.Context, _ *resend.ListOptions) (resend.ListEmailsResponse, error) {
	return resend.ListEmailsResponse{}, nil
}

func TestResendSender_SendMagicLinkEmail(t *testing.T) {
	tests := []struct {
		name            string
		isNewUser       bool
		expectedSubject string
		expectedLink    string
	}{
		{
			name:            "new user gets registration link",
			isNewUser:       true,
			expectedSubject: "Welcome to Runer — confirm your email",
			expectedLink:    "runer://auth/new?token=tok123",
		},
		{
			name:            "existing user gets sign-in link",
			isNewUser:       false,
			expectedSubject: "Your Runer sign-in link",
			expectedLink:    "runer://auth/verify?token=tok123",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubEmailsSvc{}
			sender := &ResendSender{emails: stub, fromAddr: "noreply@example.com"}

			err := sender.SendMagicLinkEmail(context.Background(), "user@example.com", "tok123", tc.isNewUser)
			require.NoError(t, err)

			req := stub.lastReq
			require.NotNil(t, req)
			assert.Equal(t, "noreply@example.com", req.From)
			assert.Equal(t, []string{"user@example.com"}, req.To)
			assert.Equal(t, tc.expectedSubject, req.Subject)
			assert.Contains(t, req.Html, tc.expectedLink)
		})
	}
}

func TestMagicLinkContent(t *testing.T) {
	subj, link := magicLinkContent(true, "abc")
	assert.Equal(t, "Welcome to Runer — confirm your email", subj)
	assert.Equal(t, "runer://auth/new?token=abc", link)

	subj, link = magicLinkContent(false, "xyz")
	assert.Equal(t, "Your Runer sign-in link", subj)
	assert.Equal(t, "runer://auth/verify?token=xyz", link)
}
