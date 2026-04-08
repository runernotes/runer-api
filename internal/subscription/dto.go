package subscription

// SubscriptionResponse is the response body for GET /api/v1/subscription.
// NoteLimit is nil when the user is on the pro plan (unlimited notes).
type SubscriptionResponse struct {
	Plan      string `json:"plan"`
	NoteCount int64  `json:"note_count"`
	NoteLimit *int   `json:"note_limit"`
}

// CheckoutResponse is the response body for POST /api/v1/subscription/checkout.
// It carries the hosted Stripe Checkout URL that the caller must redirect to.
type CheckoutResponse struct {
	CheckoutURL string `json:"checkout_url"`
}
