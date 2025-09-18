package stripe

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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

func (c *Client) ConfirmCheckout(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
	event, err := webhook.ConstructEvent(payload, signature, c.endpointSecret)
	if err != nil {
		return nil, payments.NewSignatureValidationError("payload failed signature verification", err)
	}

	var session *stripe.CheckoutSession

	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted:
		session, err = unmarshalCheckoutSession(event.Data.Raw)
		if err != nil {
			return nil, err
		}
	case stripe.EventTypeCheckoutSessionExpired:
		session, err = unmarshalCheckoutSession(event.Data.Raw)
		if err != nil {
			return nil, err
		}

		return session.Metadata, payments.NewCheckoutExpiredError("Checkout session expired")
	default:
		return nil, payments.NewNotCheckoutConfirmedEventError(fmt.Sprintf("Not a checkout session completed event. Instead got %q", event.Type))
	}

	if session.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid {
		return nil, payments.NewNotPaidError(fmt.Sprintf("Payment status is not paid. Instead got %q", session.PaymentStatus))
	}

	return session.Metadata, nil
}

func unmarshalCheckoutSession(data []byte) (*stripe.CheckoutSession, error) {
	var session stripe.CheckoutSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, payments.NewInvalidWebhookEventDataError("failed to unmarshal checkout session", err)
	}
	return &session, nil
}

func (c *Client) CreateCheckout(ctx context.Context, params payments.CheckoutParams) (payments.CheckoutInfo, error) {
	lineItems := make([]*stripe.CheckoutSessionCreateLineItemParams, len(params.Items))
	for i, item := range params.Items {
		lineItems[i] = &stripe.CheckoutSessionCreateLineItemParams{
			Quantity: stripe.Int64(int64(item.Quantity)),
			PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
				Currency:   stripe.String(item.Price.Currency().Code),
				UnitAmount: stripe.Int64(item.Price.Amount()),
				ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{
					Name: stripe.String(item.Name),
				},
			},
		}
	}

	checkoutParams := &stripe.CheckoutSessionCreateParams{
		Mode:      stripe.String(string(stripe.CheckoutSessionModePayment)),
		UIMode:    stripe.String("embedded"),
		ReturnURL: stripe.String(params.ReturnURL),
		LineItems: lineItems,
		Metadata:  params.Metadata,
		AdaptivePricing: &stripe.CheckoutSessionCreateAdaptivePricingParams{
			Enabled: stripe.Bool(params.AllowAdaptivePricing),
		},
		CustomerEmail: params.CustomerEmail,
	}

	if params.SessionAliveDuration != nil {
		checkoutParams.ExpiresAt = stripe.Int64(time.Now().Add(*params.SessionAliveDuration).Unix())
	}

	s, err := c.client.V1CheckoutSessions.Create(ctx, checkoutParams)
	if err != nil {
		return payments.CheckoutInfo{}, payments.NewFailedToCreateCheckoutSessionError("failed to create checkout session", err)
	}

	return payments.CheckoutInfo{
		ClientSecret: s.ClientSecret,
		SessionId:    s.ID,
	}, nil
}
