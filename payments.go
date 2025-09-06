package payments

import (
	"context"

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
}

type CheckoutInfo struct{}

type CheckoutManager interface {
	CreateCheckout(ctx context.Context, params CheckoutParams) (CheckoutInfo, error)
	ConfirmCheckout(ctx context.Context, payload []byte, signature string) (map[string]string, error)
}
