package payments

import (
	"testing"
	"time"

	"github.com/Rhymond/go-money"
)

func TestChargeListParams_DefaultStatus(t *testing.T) {
	params := ChargeListParams{}

	// Status should be empty by default (implementation should default to "succeeded")
	if params.Status != "" {
		t.Errorf("expected empty status by default, got %q", params.Status)
	}
}

func TestChargeListParams_WithFilters(t *testing.T) {
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)

	params := ChargeListParams{
		CreatedAfter:   &after,
		CreatedBefore:  &before,
		Status:         "succeeded",
		MetadataFilter: map[string]string{"item_type": "donation"},
	}

	if params.Status != "succeeded" {
		t.Errorf("expected status 'succeeded', got %q", params.Status)
	}

	if params.CreatedAfter == nil || !params.CreatedAfter.Equal(after) {
		t.Error("expected CreatedAfter to be set correctly")
	}

	if params.CreatedBefore == nil || !params.CreatedBefore.Equal(before) {
		t.Error("expected CreatedBefore to be set correctly")
	}

	if params.MetadataFilter["item_type"] != "donation" {
		t.Error("expected metadata filter to contain item_type: donation")
	}
}

func TestPayment_StructFields(t *testing.T) {
	addr := &Address{
		City:       "New York",
		Country:    "US",
		Line1:      "123 Main St",
		Line2:      "Apt 4B",
		PostalCode: "10001",
		State:      "NY",
	}

	billing := &BillingDetails{
		Name:    "John Doe",
		Email:   "john@example.com",
		Phone:   "+1-555-1234",
		Address: addr,
	}

	payment := Payment{
		ID:                "pi_123456",
		Amount:            money.New(1000, "USD"),
		Created:           time.Now(),
		Status:            "succeeded",
		Metadata:          map[string]string{"key": "value"},
		Description:       "Test payment",
		BillingDetails:    billing,
		CheckoutSessionID: "cs_test_123",
	}

	if payment.ID != "pi_123456" {
		t.Errorf("expected ID 'pi_123456', got %q", payment.ID)
	}

	if payment.Amount.Amount() != 1000 {
		t.Errorf("expected amount 1000, got %d", payment.Amount.Amount())
	}

	if payment.Amount.Currency().Code != "USD" {
		t.Errorf("expected currency USD, got %s", payment.Amount.Currency().Code)
	}

	if payment.Status != "succeeded" {
		t.Errorf("expected status 'succeeded', got %q", payment.Status)
	}

	if payment.BillingDetails == nil {
		t.Fatal("expected BillingDetails to be set")
	}

	if payment.BillingDetails.Name != "John Doe" {
		t.Errorf("expected billing name 'John Doe', got %q", payment.BillingDetails.Name)
	}

	if payment.BillingDetails.Address == nil {
		t.Fatal("expected Address to be set")
	}

	if payment.BillingDetails.Address.State != "NY" {
		t.Errorf("expected state 'NY', got %q", payment.BillingDetails.Address.State)
	}

	if payment.CheckoutSessionID != "cs_test_123" {
		t.Errorf("expected checkout session ID 'cs_test_123', got %q", payment.CheckoutSessionID)
	}
}

func TestAddress_StructFields(t *testing.T) {
	addr := Address{
		City:       "San Francisco",
		Country:    "US",
		Line1:      "456 Market St",
		Line2:      "",
		PostalCode: "94102",
		State:      "CA",
	}

	if addr.City != "San Francisco" {
		t.Errorf("expected city 'San Francisco', got %q", addr.City)
	}

	if addr.State != "CA" {
		t.Errorf("expected state 'CA', got %q", addr.State)
	}

	if addr.PostalCode != "94102" {
		t.Errorf("expected postal code '94102', got %q", addr.PostalCode)
	}
}

func TestBillingDetails_StructFields(t *testing.T) {
	billing := BillingDetails{
		Name:    "Jane Smith",
		Email:   "jane@example.com",
		Phone:   "+1-555-5678",
		Address: nil,
	}

	if billing.Name != "Jane Smith" {
		t.Errorf("expected name 'Jane Smith', got %q", billing.Name)
	}

	if billing.Email != "jane@example.com" {
		t.Errorf("expected email 'jane@example.com', got %q", billing.Email)
	}

	if billing.Address != nil {
		t.Error("expected Address to be nil")
	}
}
