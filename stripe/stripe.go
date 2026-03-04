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
	"github.com/stripe/stripe-go/v82/charge"
	"github.com/stripe/stripe-go/v82/paymentintent"
	"github.com/stripe/stripe-go/v82/webhook"
)

var _ payments.CheckoutManager = &Client{}
var _ payments.PaymentQuerier = &Client{}

type Client struct {
	client         *stripe.Client
	secretKey      string
	endpointSecret string
}

func NewClient(secretKey string, endpointSecret string) *Client {
	sc := stripe.NewClient(secretKey)

	return &Client{
		client:         sc,
		secretKey:      secretKey,
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
			searchParams := buildPaymentIntentSearchParams(convertToPaginatedParams(params, 100))
			for pi, err := range c.client.V1PaymentIntents.Search(ctx, searchParams) {
				if err != nil {
					yield(payments.Payment{}, err)
					return
				}

				// Get the latest charge from the PaymentIntent
				if pi.LatestCharge != nil && pi.LatestCharge.ID != "" {
					payment := convertPaymentIntentToPayment(pi)
					if !yield(payment, nil) {
						return
					}
				}
			}
			return
		}

		// Use List API when no metadata filtering needed
		listParams := buildListParams(convertToPaginatedParams(params, 100))
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

func (c *Client) ListChargesPaginated(ctx context.Context, params payments.ChargeListPaginatedParams) (payments.ChargesPage, error) {
	// Set default limit if not specified
	limit := params.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	// When metadata filtering is needed, search PaymentIntents instead
	// (metadata is stored on PaymentIntent, not Charge)
	if len(params.MetadataFilter) > 0 {
		return c.listChargesPaginatedFromSearch(ctx, params, limit)
	}

	// Use List API when no metadata filtering needed
	return c.listChargesPaginatedFromList(ctx, params, limit)
}

func (c *Client) listChargesPaginatedFromSearch(ctx context.Context, params payments.ChargeListPaginatedParams, limit int) (payments.ChargesPage, error) {
	// TODO: Use the new stripe.Client pattern once pagination properties are exposed.
	// Currently the new client doesn't expose has_more/next_page (see: https://github.com/stripe/stripe-go/issues/2168).
	// For now, we use the deprecated paymentintent.Client which still exposes these properties.
	params.Limit = limit
	searchParams := buildPaymentIntentSearchParams(params)
	if params.Cursor != "" {
		searchParams.Page = stripe.String(params.Cursor)
	}

	// Create deprecated client with our API key and backend
	piClient := &paymentintent.Client{
		B:   stripe.GetBackend(stripe.APIBackend),
		Key: c.secretKey,
	}

	// Use deprecated resource-specific client to get pagination info
	iter := piClient.Search(searchParams)

	result := payments.ChargesPage{
		Payments: make([]payments.Payment, 0, limit),
	}

	// Convert PaymentIntents to Payments
	searchResult := iter.PaymentIntentSearchResult()
	for _, pi := range searchResult.Data {
		if pi.LatestCharge != nil && pi.LatestCharge.ID != "" {
			payment := convertPaymentIntentToPayment(pi)
			result.Payments = append(result.Payments, payment)
		}
	}

	// Set pagination info from response
	result.HasMore = searchResult.HasMore
	if searchResult.NextPage != nil && *searchResult.NextPage != "" {
		result.NextCursor = *searchResult.NextPage
	}

	return result, nil
}

func (c *Client) listChargesPaginatedFromList(ctx context.Context, params payments.ChargeListPaginatedParams, limit int) (payments.ChargesPage, error) {
	// TODO: Use the new stripe.Client pattern once pagination properties are exposed.
	// Currently the new client doesn't expose has_more (see: https://github.com/stripe/stripe-go/issues/2168).
	// For now, we use the deprecated charge.Client which still exposes these properties.
	params.Limit = limit
	listParams := buildListParams(params)

	// Create deprecated client with our API key and backend
	chargeClient := &charge.Client{
		B:   stripe.GetBackend(stripe.APIBackend),
		Key: c.secretKey,
	}

	// Use deprecated resource-specific client to get pagination info
	iter := chargeClient.List(listParams)

	result := payments.ChargesPage{
		Payments: make([]payments.Payment, 0, limit),
	}

	// Convert Charges to Payments
	chargeList := iter.ChargeList()
	for _, ch := range chargeList.Data {
		payment := convertChargeToPayment(ch)
		result.Payments = append(result.Payments, payment)
	}

	// Set pagination info from response
	result.HasMore = chargeList.HasMore
	if chargeList.HasMore && len(chargeList.Data) > 0 {
		// Use the last charge ID as the cursor
		lastCharge := chargeList.Data[len(chargeList.Data)-1]
		result.NextCursor = lastCharge.ID
	}

	return result, nil
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

func buildPaymentIntentSearchParams(params payments.ChargeListPaginatedParams) *stripe.PaymentIntentSearchParams {
	searchParams := &stripe.PaymentIntentSearchParams{}

	// Set limit
	searchParams.Limit = stripe.Int64(int64(params.Limit))

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

func buildListParams(params payments.ChargeListPaginatedParams) *stripe.ChargeListParams {
	listParams := &stripe.ChargeListParams{}

	// Set limit
	listParams.Filters.AddFilter("limit", "", fmt.Sprintf("%d", params.Limit))

	// Set starting_after cursor if provided
	if params.Cursor != "" {
		listParams.Filters.AddFilter("starting_after", "", params.Cursor)
	}

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

// convertToPaginatedParams converts ChargeListParams to ChargeListPaginatedParams
func convertToPaginatedParams(params payments.ChargeListParams, limit int) payments.ChargeListPaginatedParams {
	return payments.ChargeListPaginatedParams{
		CreatedAfter:   params.CreatedAfter,
		CreatedBefore:  params.CreatedBefore,
		Status:         params.Status,
		MetadataFilter: params.MetadataFilter,
		Limit:          limit,
	}
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
	var ch *stripe.Charge
	if pi.LatestCharge != nil && pi.LatestCharge.ID != "" {
		ch = pi.LatestCharge
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
	if ch != nil {
		payment.ID = ch.ID
		payment.Amount = money.New(ch.Amount, string(ch.Currency))
		payment.Created = time.Unix(ch.Created, 0)
		payment.Status = string(ch.Status)
		payment.Description = ch.Description

		if ch.BillingDetails != nil {
			payment.BillingDetails = &payments.BillingDetails{
				Name:  ch.BillingDetails.Name,
				Email: ch.BillingDetails.Email,
				Phone: ch.BillingDetails.Phone,
			}

			if ch.BillingDetails.Address != nil {
				payment.BillingDetails.Address = &payments.Address{
					City:       ch.BillingDetails.Address.City,
					Country:    ch.BillingDetails.Address.Country,
					Line1:      ch.BillingDetails.Address.Line1,
					Line2:      ch.BillingDetails.Address.Line2,
					PostalCode: ch.BillingDetails.Address.PostalCode,
					State:      ch.BillingDetails.Address.State,
				}
			}
		}
	}

	return payment
}
