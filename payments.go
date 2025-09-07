package payments

import (
	"context"
	"time"

	"github.com/Rhymond/go-money"
)

type Item struct {
	Name  string
	Price *money.Money
}

type CheckoutParams struct {
	ReturnURL string
	Items     []Item
	Metadata  map[string]string
	// How long to keep the checkout session alive.
	// Check payment operator for allowed values.
	SessionAliveDuration *time.Duration
	// If the payment processor has an adaptive pricing feature (i.e. auto converting currencies),
	// enable or disable it.
	AllowAdaptivePricing bool
}

type CheckoutInfo struct {
	ClientSecret string
	SessionId    string
}

type CheckoutManager interface {
	CreateCheckout(ctx context.Context, params CheckoutParams) (CheckoutInfo, error)
	ConfirmCheckout(ctx context.Context, payload []byte, signature string) (map[string]string, error)
}
