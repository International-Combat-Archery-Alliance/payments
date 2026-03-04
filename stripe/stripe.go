package stripe

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"time"

	"github.com/International-Combat-Archery-Alliance/payments"
	"github.com/Rhymond/go-money"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

var _ payments.CheckoutManager = &Client{}
var _ payments.PaymentQuerier = &Client{}

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

	if params.ReceiptEmail != nil || len(params.Metadata) > 0 {
		checkoutParams.PaymentIntentData = &stripe.CheckoutSessionCreatePaymentIntentDataParams{
			ReceiptEmail: params.ReceiptEmail,
		}
		// Copy metadata to PaymentIntent so it can be searched
		if len(params.Metadata) > 0 {
			checkoutParams.PaymentIntentData.Metadata = params.Metadata
		}
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

func (c *Client) ListCharges(ctx context.Context, params payments.ChargeListParams) iter.Seq2[payments.Payment, error] {
	return func(yield func(payments.Payment, error) bool) {
		// When metadata filtering is needed, search PaymentIntents instead
		// (metadata is stored on PaymentIntent, not Charge)
		if len(params.MetadataFilter) > 0 {
			searchParams := buildPaymentIntentSearchParams(params)
			for pi, err := range c.client.V1PaymentIntents.Search(ctx, searchParams) {
				if err != nil {
					yield(payments.Payment{}, err)
					return
				}

				// Get the latest charge from the PaymentIntent
				if pi.LatestCharge != nil && pi.LatestCharge.ID != "" {
					// Try to get the full charge data if expanded
					payment := convertPaymentIntentToPayment(pi)
					if !yield(payment, nil) {
						return
					}
				}
			}
			return
		}

		// Use List API when no metadata filtering needed
		listParams := buildListParams(params)
		for charge, err := range c.client.V1Charges.List(ctx, listParams) {
			if err != nil {
				yield(payments.Payment{}, err)
				return
			}

			payment := convertChargeToPayment(charge)
			if !yield(payment, nil) {
				return
			}
		}
	}
}

func buildPaymentIntentSearchParams(params payments.ChargeListParams) *stripe.PaymentIntentSearchParams {
	searchParams := &stripe.PaymentIntentSearchParams{}

	// Expand data.latest_charge to get full charge data including billing_details
	searchParams.AddExpand("data.latest_charge")

	// Build query string
	queryParts := []string{}

	// Add status filter (default to succeeded)
	status := params.Status
	if status == "" {
		status = string(stripe.PaymentIntentStatusSucceeded)
	}
	queryParts = append(queryParts, fmt.Sprintf("status:'%s'", status))

	// Add date range filters
	if params.CreatedAfter != nil {
		queryParts = append(queryParts, fmt.Sprintf("created>%d", params.CreatedAfter.Unix()))
	}
	if params.CreatedBefore != nil {
		queryParts = append(queryParts, fmt.Sprintf("created<%d", params.CreatedBefore.Unix()))
	}

	// Add metadata filters
	for key, value := range params.MetadataFilter {
		queryParts = append(queryParts, fmt.Sprintf("metadata['%s']:'%s'", key, value))
	}

	searchParams.Query = joinWithAnd(queryParts)
	return searchParams
}

func joinWithAnd(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result = result + " AND " + parts[i]
	}
	return result
}

func buildListParams(params payments.ChargeListParams) *stripe.ChargeListParams {
	listParams := &stripe.ChargeListParams{}

	// Expand data.payment_intent to get metadata from PaymentIntent
	// (must use 'data.' prefix for expansions in list operations)
	listParams.AddExpand("data.payment_intent")

	// Add status filter (default to succeeded)
	status := params.Status
	if status == "" {
		status = string(stripe.ChargeStatusSucceeded)
	}
	listParams.Filters.AddFilter("status", "", status)

	// Add date range filters
	if params.CreatedAfter != nil {
		listParams.Filters.AddFilter("created[gte]", "", fmt.Sprintf("%d", params.CreatedAfter.Unix()))
	}
	if params.CreatedBefore != nil {
		listParams.Filters.AddFilter("created[lte]", "", fmt.Sprintf("%d", params.CreatedBefore.Unix()))
	}

	return listParams
}

func convertChargeToPayment(charge *stripe.Charge) payments.Payment {
	// Merge metadata from Charge and PaymentIntent (PaymentIntent metadata takes precedence)
	mergedMetadata := make(map[string]string)

	// First, copy Charge metadata
	for k, v := range charge.Metadata {
		mergedMetadata[k] = v
	}

	// Then, copy PaymentIntent metadata (overwrites Charge metadata if keys overlap)
	if charge.PaymentIntent != nil {
		for k, v := range charge.PaymentIntent.Metadata {
			mergedMetadata[k] = v
		}
	}

	payment := payments.Payment{
		ID:          charge.ID,
		Amount:      money.New(charge.Amount, string(charge.Currency)),
		Created:     time.Unix(charge.Created, 0),
		Status:      string(charge.Status),
		Metadata:    mergedMetadata,
		Description: charge.Description,
	}

	if charge.BillingDetails != nil {
		payment.BillingDetails = &payments.BillingDetails{
			Name:  charge.BillingDetails.Name,
			Email: charge.BillingDetails.Email,
			Phone: charge.BillingDetails.Phone,
		}

		if charge.BillingDetails.Address != nil {
			payment.BillingDetails.Address = &payments.Address{
				City:       charge.BillingDetails.Address.City,
				Country:    charge.BillingDetails.Address.Country,
				Line1:      charge.BillingDetails.Address.Line1,
				Line2:      charge.BillingDetails.Address.Line2,
				PostalCode: charge.BillingDetails.Address.PostalCode,
				State:      charge.BillingDetails.Address.State,
			}
		}
	}

	if charge.PaymentIntent != nil {
		payment.CheckoutSessionID = extractCheckoutSessionID(charge.PaymentIntent)
	}

	return payment
}

func extractCheckoutSessionID(pi *stripe.PaymentIntent) string {
	if pi == nil {
		return ""
	}
	// PaymentIntent metadata often contains checkout_session_id
	if sessionID, ok := pi.Metadata["checkout_session_id"]; ok {
		return sessionID
	}
	return ""
}

func convertPaymentIntentToPayment(pi *stripe.PaymentIntent) payments.Payment {
	// Use the expanded latest_charge data if available
	var charge *stripe.Charge
	if pi.LatestCharge != nil && pi.LatestCharge.ID != "" {
		charge = pi.LatestCharge
	}

	// Build the payment from PaymentIntent data
	payment := payments.Payment{
		ID:                pi.ID,
		Amount:            money.New(pi.Amount, string(pi.Currency)),
		Created:           time.Unix(pi.Created, 0),
		Status:            string(pi.Status),
		Metadata:          pi.Metadata,
		Description:       pi.Description,
		CheckoutSessionID: extractCheckoutSessionID(pi),
	}

	// If we have expanded charge data, use it for additional fields
	if charge != nil {
		payment.ID = charge.ID
		payment.Amount = money.New(charge.Amount, string(charge.Currency))
		payment.Created = time.Unix(charge.Created, 0)
		payment.Status = string(charge.Status)
		payment.Description = charge.Description

		if charge.BillingDetails != nil {
			payment.BillingDetails = &payments.BillingDetails{
				Name:  charge.BillingDetails.Name,
				Email: charge.BillingDetails.Email,
				Phone: charge.BillingDetails.Phone,
			}

			if charge.BillingDetails.Address != nil {
				payment.BillingDetails.Address = &payments.Address{
					City:       charge.BillingDetails.Address.City,
					Country:    charge.BillingDetails.Address.Country,
					Line1:      charge.BillingDetails.Address.Line1,
					Line2:      charge.BillingDetails.Address.Line2,
					PostalCode: charge.BillingDetails.Address.PostalCode,
					State:      charge.BillingDetails.Address.State,
				}
			}
		}
	}

	return payment
}
