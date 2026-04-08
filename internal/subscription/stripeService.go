package subscription

import (
	"context"
	"errors"
	"fmt"

	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/customer"
)

// StripeClient is the minimal surface the checkout handler needs from Stripe.
// It exists as an interface so the handler can be tested without any network or
// SDK dependency — tests provide a fake implementation.
type StripeClient interface {
	// CreateCustomer provisions a new Stripe customer for the given email and
	// returns the new customer id.
	CreateCustomer(ctx context.Context, email string) (string, error)

	// CreateCheckoutSession creates a Stripe Checkout session in subscription
	// mode for the given customer and price, using the configured success /
	// cancel URLs, and returns the hosted checkout URL.
	CreateCheckoutSession(ctx context.Context, params CheckoutSessionParams) (string, error)
}

// CheckoutSessionParams carries the inputs required to create a checkout session.
type CheckoutSessionParams struct {
	CustomerID string
	PriceID    string
	SuccessURL string
	CancelURL  string
}

// stripeSDKClient is the production StripeClient implementation backed by
// github.com/stripe/stripe-go/v81. It reads the API key on construction.
type stripeSDKClient struct {
	apiKey string
}

// NewStripeClient returns a StripeClient that talks to the real Stripe API.
// It panics if apiKey is empty — the caller (route wiring) is responsible for
// only constructing this when BILLING_ENABLED=true with valid config.
func NewStripeClient(apiKey string) StripeClient {
	if apiKey == "" {
		panic("subscription: stripe api key must not be empty")
	}
	return &stripeSDKClient{apiKey: apiKey}
}

func (c *stripeSDKClient) CreateCustomer(ctx context.Context, email string) (string, error) {
	if email == "" {
		return "", errors.New("subscription: email must not be empty")
	}
	// stripe-go reads the key from a package-global; set it per call for safety.
	// The SDK is safe to use this way — the key is effectively immutable once
	// NewStripeClient is constructed from validated config.
	stripe.Key = c.apiKey

	params := &stripe.CustomerParams{Email: stripe.String(email)}
	params.Context = ctx

	cust, err := customer.New(params)
	if err != nil {
		return "", fmt.Errorf("subscription: create stripe customer: %w", err)
	}
	return cust.ID, nil
}

func (c *stripeSDKClient) CreateCheckoutSession(ctx context.Context, p CheckoutSessionParams) (string, error) {
	if p.CustomerID == "" {
		return "", errors.New("subscription: customer id must not be empty")
	}
	if p.PriceID == "" {
		return "", errors.New("subscription: price id must not be empty")
	}
	stripe.Key = c.apiKey

	params := &stripe.CheckoutSessionParams{
		Mode:     stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		Customer: stripe.String(p.CustomerID),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{Price: stripe.String(p.PriceID), Quantity: stripe.Int64(1)},
		},
		SuccessURL: stripe.String(p.SuccessURL),
		CancelURL:  stripe.String(p.CancelURL),
	}
	params.Context = ctx

	s, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("subscription: create checkout session: %w", err)
	}
	return s.URL, nil
}
