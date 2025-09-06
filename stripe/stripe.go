package stripe

import (
	"context"
	"fmt"

	"github.com/International-Combat-Archery-Alliance/payments"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

var _ payments.CheckoutManager = &Client{}

type Client struct {
	client         *stripe.Client
	endpointSecret string
}

func NewClient(secretKey string, endpointSecret string) *Client {
	sc := stripe.NewClient(secretKey)

	return &Client{
		client:         sc,
		endpointSecret: endpointSecret,
	}
}

// ConfirmCheckout implements payments.CheckoutManager.
func (c *Client) ConfirmCheckout(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
	event, err := webhook.ConstructEvent(payload, signature, c.endpointSecret)
	if err != nil {
		return nil, fmt.Errorf("payload failed signature verification: %w", err)
	}

	// TODO: check event and get metadata out
	return map[string]string{}, nil
}

// CreateCheckout implements payments.CheckoutManager.
func (c *Client) CreateCheckout(ctx context.Context, params payments.CheckoutParams) (payments.CheckoutInfo, error) {
	lineItems := make([]*stripe.CheckoutSessionCreateLineItemParams, len(params.Items))
	for i, item := range params.Items {
		lineItems[i] = &stripe.CheckoutSessionCreateLineItemParams{
			PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
				Currency:   stripe.String(item.Price.Currency().Code),
				UnitAmount: stripe.Int64(item.Price.Amount()),
				ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{
					Name: stripe.String(item.Name),
				},
			},
		}
	}

	s, err := c.client.V1CheckoutSessions.Create(ctx, &stripe.CheckoutSessionCreateParams{
		Mode:      stripe.String(string(stripe.CheckoutSessionModePayment)),
		UIMode:    stripe.String("embedded"),
		ReturnURL: stripe.String(params.ReturnURL),
		LineItems: lineItems,
	})
	if err != nil {
		return payments.CheckoutInfo{}, fmt.Errorf("failed to create checkout session: %w", err)
	}

	return payments.CheckoutInfo{}, nil
}
