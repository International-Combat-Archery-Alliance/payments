package stripe

import (
	"strings"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/payments"
	"github.com/stripe/stripe-go/v82"
)

func TestBuildPaymentIntentSearchParams_WithMetadata(t *testing.T) {
	params := payments.ChargeListParams{
		MetadataFilter: map[string]string{
			"item_type": "donation",
			"campaign":  "2024",
		},
	}

	searchParams := buildPaymentIntentSearchParams(params)

	// Query should contain metadata filters
	if !strings.Contains(searchParams.Query, "metadata['item_type']:'donation'") {
		t.Errorf("expected query to contain metadata item_type filter, got: %s", searchParams.Query)
	}

	if !strings.Contains(searchParams.Query, "metadata['campaign']:'2024'") {
		t.Errorf("expected query to contain metadata campaign filter, got: %s", searchParams.Query)
	}

	// Query should contain status filter (default to succeeded)
	if !strings.Contains(searchParams.Query, "status:'succeeded'") {
		t.Errorf("expected query to contain status filter, got: %s", searchParams.Query)
	}
}

func TestBuildPaymentIntentSearchParams_WithDateRange(t *testing.T) {
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	params := payments.ChargeListParams{
		CreatedAfter:  &after,
		CreatedBefore: &before,
	}

	searchParams := buildPaymentIntentSearchParams(params)

	// Query should contain date filters
	if !strings.Contains(searchParams.Query, "created>1704067200") {
		t.Errorf("expected query to contain created after filter, got: %s", searchParams.Query)
	}

	if !strings.Contains(searchParams.Query, "created<1735603200") {
		t.Errorf("expected query to contain created before filter, got: %s", searchParams.Query)
	}
}

func TestBuildPaymentIntentSearchParams_WithCustomStatus(t *testing.T) {
	params := payments.ChargeListParams{
		Status:         "requires_capture",
		MetadataFilter: map[string]string{"item_type": "donation"},
	}

	searchParams := buildPaymentIntentSearchParams(params)

	if !strings.Contains(searchParams.Query, "status:'requires_capture'") {
		t.Errorf("expected query to contain status:'requires_capture', got: %s", searchParams.Query)
	}
}

func TestJoinWithAnd(t *testing.T) {
	// Test empty slice
	result := joinWithAnd([]string{})
	if result != "" {
		t.Errorf("expected empty string for empty slice, got: %s", result)
	}

	// Test single element
	result = joinWithAnd([]string{"status:'succeeded'"})
	if result != "status:'succeeded'" {
		t.Errorf("expected 'status:'succeeded'', got: %s", result)
	}

	// Test multiple elements
	result = joinWithAnd([]string{"status:'succeeded'", "metadata['key']:'value'"})
	expected := "status:'succeeded' AND metadata['key']:'value'"
	if result != expected {
		t.Errorf("expected '%s', got: %s", expected, result)
	}

	// Test three elements
	result = joinWithAnd([]string{"a", "b", "c"})
	expected = "a AND b AND c"
	if result != expected {
		t.Errorf("expected '%s', got: %s", expected, result)
	}
}

func TestBuildListParams_WithDateRange(t *testing.T) {
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	params := payments.ChargeListParams{
		CreatedAfter:  &after,
		CreatedBefore: &before,
	}

	listParams := buildListParams(params)

	// Verify the ListParams is created (we can't inspect internal Filters directly)
	// but we can verify it compiles and runs
	if listParams == nil {
		t.Error("expected listParams to be non-nil")
	}
}

func TestConvertChargeToPayment_BasicFields(t *testing.T) {
	charge := &stripe.Charge{
		ID:          "ch_123456",
		Amount:      2000,
		Currency:    "usd",
		Created:     time.Now().Unix(),
		Status:      stripe.ChargeStatusSucceeded,
		Metadata:    map[string]string{"source": "test"},
		Description: "Test charge",
	}

	payment := convertChargeToPayment(charge)

	if payment.ID != "ch_123456" {
		t.Errorf("expected ID 'ch_123456', got %q", payment.ID)
	}

	if payment.Amount.Amount() != 2000 {
		t.Errorf("expected amount 2000, got %d", payment.Amount.Amount())
	}

	if payment.Amount.Currency().Code != "USD" {
		t.Errorf("expected currency USD, got %s", payment.Amount.Currency().Code)
	}

	if payment.Status != string(stripe.ChargeStatusSucceeded) {
		t.Errorf("expected status 'succeeded', got %q", payment.Status)
	}

	if payment.Description != "Test charge" {
		t.Errorf("expected description 'Test charge', got %q", payment.Description)
	}

	if payment.Metadata["source"] != "test" {
		t.Error("expected metadata to be preserved")
	}
}

func TestConvertChargeToPayment_WithBillingDetails(t *testing.T) {
	charge := &stripe.Charge{
		ID:       "ch_789",
		Amount:   5000,
		Currency: "eur",
		BillingDetails: &stripe.ChargeBillingDetails{
			Name:  "Test User",
			Email: "test@example.com",
			Phone: "+1-555-0000",
			Address: &stripe.Address{
				City:       "Berlin",
				Country:    "DE",
				Line1:      "Test Str 1",
				Line2:      "Apt 2",
				PostalCode: "10115",
				State:      "BE",
			},
		},
	}

	payment := convertChargeToPayment(charge)

	if payment.BillingDetails == nil {
		t.Fatal("expected BillingDetails to be set")
	}

	if payment.BillingDetails.Name != "Test User" {
		t.Errorf("expected name 'Test User', got %q", payment.BillingDetails.Name)
	}

	if payment.BillingDetails.Email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got %q", payment.BillingDetails.Email)
	}

	if payment.BillingDetails.Phone != "+1-555-0000" {
		t.Errorf("expected phone '+1-555-0000', got %q", payment.BillingDetails.Phone)
	}

	if payment.BillingDetails.Address == nil {
		t.Fatal("expected Address to be set")
	}

	if payment.BillingDetails.Address.City != "Berlin" {
		t.Errorf("expected city 'Berlin', got %q", payment.BillingDetails.Address.City)
	}

	if payment.BillingDetails.Address.State != "BE" {
		t.Errorf("expected state 'BE', got %q", payment.BillingDetails.Address.State)
	}

	if payment.BillingDetails.Address.Country != "DE" {
		t.Errorf("expected country 'DE', got %q", payment.BillingDetails.Address.Country)
	}
}

func TestConvertChargeToPayment_NoBillingDetails(t *testing.T) {
	charge := &stripe.Charge{
		ID:             "ch_simple",
		Amount:         1000,
		Currency:       "usd",
		BillingDetails: nil,
	}

	payment := convertChargeToPayment(charge)

	if payment.BillingDetails != nil {
		t.Error("expected BillingDetails to be nil when charge has no billing details")
	}
}

func TestConvertChargeToPayment_WithPaymentIntent(t *testing.T) {
	pi := &stripe.PaymentIntent{
		ID:       "pi_123",
		Metadata: map[string]string{"checkout_session_id": "cs_test_abc"},
	}

	charge := &stripe.Charge{
		ID:            "ch_with_pi",
		Amount:        3000,
		Currency:      "usd",
		PaymentIntent: pi,
	}

	payment := convertChargeToPayment(charge)

	if payment.CheckoutSessionID != "cs_test_abc" {
		t.Errorf("expected checkout session ID 'cs_test_abc', got %q", payment.CheckoutSessionID)
	}
}

func TestConvertChargeToPayment_NoPaymentIntent(t *testing.T) {
	charge := &stripe.Charge{
		ID:       "ch_no_pi",
		Amount:   1000,
		Currency: "usd",
	}

	payment := convertChargeToPayment(charge)

	if payment.CheckoutSessionID != "" {
		t.Errorf("expected empty checkout session ID, got %q", payment.CheckoutSessionID)
	}
}

func TestConvertChargeToPayment_PaymentIntentNoMetadata(t *testing.T) {
	pi := &stripe.PaymentIntent{
		ID:       "pi_no_meta",
		Metadata: map[string]string{},
	}

	charge := &stripe.Charge{
		ID:            "ch_pi_no_meta",
		Amount:        1000,
		Currency:      "usd",
		PaymentIntent: pi,
	}

	payment := convertChargeToPayment(charge)

	if payment.CheckoutSessionID != "" {
		t.Errorf("expected empty checkout session ID when metadata is empty, got %q", payment.CheckoutSessionID)
	}
}

func TestExtractCheckoutSessionID_WithSessionID(t *testing.T) {
	pi := &stripe.PaymentIntent{
		Metadata: map[string]string{"checkout_session_id": "cs_test_xyz"},
	}

	sessionID := extractCheckoutSessionID(pi)

	if sessionID != "cs_test_xyz" {
		t.Errorf("expected 'cs_test_xyz', got %q", sessionID)
	}
}

func TestExtractCheckoutSessionID_NilPaymentIntent(t *testing.T) {
	sessionID := extractCheckoutSessionID(nil)

	if sessionID != "" {
		t.Errorf("expected empty string for nil PaymentIntent, got %q", sessionID)
	}
}

func TestExtractCheckoutSessionID_NoSessionID(t *testing.T) {
	pi := &stripe.PaymentIntent{
		Metadata: map[string]string{"other_key": "value"},
	}

	sessionID := extractCheckoutSessionID(pi)

	if sessionID != "" {
		t.Errorf("expected empty string when checkout_session_id not present, got %q", sessionID)
	}
}

func TestExtractCheckoutSessionID_EmptyMetadata(t *testing.T) {
	pi := &stripe.PaymentIntent{
		Metadata: map[string]string{},
	}

	sessionID := extractCheckoutSessionID(pi)

	if sessionID != "" {
		t.Errorf("expected empty string for empty metadata, got %q", sessionID)
	}
}

// Test that Client implements PaymentQuerier interface
func TestClient_ImplementsPaymentQuerier(t *testing.T) {
	client := &Client{}
	var _ payments.PaymentQuerier = client
}
