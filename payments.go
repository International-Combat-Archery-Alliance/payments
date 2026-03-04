package payments

import (
	"context"
	"iter"
	"time"

	"github.com/Rhymond/go-money"
)

type Item struct {
	Name     string
	Price    *money.Money
	Quantity int
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
	CustomerEmail        *string
	// Email address where the payment provider should send the payment receipt.
	ReceiptEmail *string
}

type CheckoutInfo struct {
	ClientSecret string
	SessionId    string
}

type CheckoutManager interface {
	CreateCheckout(ctx context.Context, params CheckoutParams) (CheckoutInfo, error)
	// Confirms that a checkout was sucessful based on event data.
	//
	// Note that this can still return metadata even if err != nil, in the event that the
	// checkout was expired, since the caller may still want to know checkout information.
	ConfirmCheckout(ctx context.Context, payload []byte, signature string) (map[string]string, error)
}

type Address struct {
	City       string
	Country    string
	Line1      string
	Line2      string
	PostalCode string
	State      string
}

type BillingDetails struct {
	Name    string
	Email   string
	Phone   string
	Address *Address
}

type Payment struct {
	ID                string
	Amount            *money.Money
	Created           time.Time
	Status            string
	Metadata          map[string]string
	Description       string
	BillingDetails    *BillingDetails
	CheckoutSessionID string
}

type ChargeListParams struct {
	CreatedAfter   *time.Time
	CreatedBefore  *time.Time
	Status         string
	MetadataFilter map[string]string
}

type ChargeListPaginatedParams struct {
	CreatedAfter   *time.Time
	CreatedBefore  *time.Time
	Status         string
	MetadataFilter map[string]string
	Limit          int    // max items per page. Defaults to 10 if not set.
	Cursor         string // ID of last item from previous page (empty for first page)
}

type ChargesPage struct {
	Payments   []Payment
	HasMore    bool
	NextCursor string // ID to use for next page request (empty if HasMore=false)
}

type PaymentQuerier interface {
	ListCharges(ctx context.Context, params ChargeListParams) iter.Seq2[Payment, error]
	ListChargesPaginated(ctx context.Context, params ChargeListPaginatedParams) (ChargesPage, error)
}
