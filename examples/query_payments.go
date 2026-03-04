// Example program demonstrating how to use the payments library to query and aggregate charges.
//
// Usage:
//
//	export STRIPE_SECRET_KEY=sk_test_...
//	go run examples/query_payments.go
//
// Note: This is a test example. Do not use real credentials in production code.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/International-Combat-Archery-Alliance/payments"
	"github.com/International-Combat-Archery-Alliance/payments/stripe"
	gomoney "github.com/Rhymond/go-money"
)

func main() {
	// Get credentials from environment
	secretKey := os.Getenv("STRIPE_SECRET_KEY")
	if secretKey == "" {
		log.Fatal("STRIPE_SECRET_KEY environment variable is required")
	}

	// Create Stripe client
	client := stripe.NewClient(secretKey, "")

	// Demonstrate PaymentQuerier usage
	ctx := context.Background()

	fmt.Println("=== Example 1: List all successful charges from last 30 days ===")
	listRecentCharges(ctx, client)

	fmt.Println("\n=== Example 2: Filter charges by metadata ===")
	listDonationsByMetadata(ctx, client)

	fmt.Println("\n=== Example 3: Aggregate donations by state ===")
	aggregateDonationsByState(ctx, client)
}

// listRecentCharges demonstrates basic charge listing with date range
func listRecentCharges(ctx context.Context, querier payments.PaymentQuerier) {
	// Get charges from last 30 days
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	params := payments.ChargeListParams{
		CreatedAfter: &thirtyDaysAgo,
		Status:       "succeeded",
	}

	count := 0
	var totalMoney *gomoney.Money

	for payment, err := range querier.ListCharges(ctx, params) {
		if err != nil {
			log.Printf("Error fetching charge: %v", err)
			continue
		}

		count++

		// Initialize or add to total
		if totalMoney == nil {
			totalMoney = payment.Amount
		} else {
			totalMoney, err = totalMoney.Add(payment.Amount)
			if err != nil {
				// Different currencies - start a new total
				log.Printf("Warning: mixed currencies detected (%s vs %s)",
					totalMoney.Currency().Code, payment.Amount.Currency().Code)
				// For simplicity, we'll just track the first currency's total
				// In production, you'd want to group by currency
			}
		}

		// Print first 5 charges as example
		if count <= 5 {
			fmt.Printf("  Charge %s: %s (Created: %s %+v)\n",
				payment.ID,
				payment.Amount.Display(),
				payment.Created.Format("2006-01-02"),
				payment.Metadata,
			)
		}
	}

	if totalMoney != nil {
		fmt.Printf("  ... Total: %d charges, %s\n", count, totalMoney.Display())
	} else {
		fmt.Printf("  ... Total: %d charges\n", count)
	}
}

// listDonationsByMetadata demonstrates filtering by metadata
func listDonationsByMetadata(ctx context.Context, querier payments.PaymentQuerier) {
	// Search for donations with specific metadata
	params := payments.ChargeListParams{
		MetadataFilter: map[string]string{
			"item_type": "donation",
		},
		Status: "succeeded",
	}

	fmt.Println("  Searching for donations with metadata:")
	fmt.Printf("    item_type: 'donation'\n")
	fmt.Println()
	fmt.Println("  Note: Metadata is merged from both Charge and PaymentIntent objects")
	fmt.Println("  (PaymentIntent metadata takes precedence if keys overlap)")
	fmt.Println()

	count := 0
	for payment, err := range querier.ListCharges(ctx, params) {
		if err != nil {
			log.Printf("Error fetching charge: %v", err)
			continue
		}

		count++
		if count <= 3 {
			fmt.Printf("  Found donation %s: %s\n", payment.ID, payment.Amount.Display())
			fmt.Printf("    Checkout Session ID: %s\n", payment.CheckoutSessionID)
			fmt.Printf("    All Metadata: %v\n", payment.Metadata)
			if val, ok := payment.Metadata["item_type"]; ok {
				fmt.Printf("    item_type value: '%s'\n", val)
			}
			fmt.Println()
		}
	}

	fmt.Printf("  Total matching donations: %d\n", count)
}

// aggregateDonationsByState demonstrates the original use case:
// aggregating payments by billing state
func aggregateDonationsByState(ctx context.Context, querier payments.PaymentQuerier) {
	// Get all donation charges
	params := payments.ChargeListParams{
		MetadataFilter: map[string]string{
			"item_type": "donation",
		},
		Status: "succeeded",
	}

	// Map to aggregate by state and currency
	type stateCurrencyKey struct {
		state    string
		currency string
	}
	stateTotals := make(map[stateCurrencyKey]*gomoney.Money)
	stateCounts := make(map[stateCurrencyKey]int)

	for payment, err := range querier.ListCharges(ctx, params) {
		if err != nil {
			log.Printf("Error fetching charge: %v", err)
			continue
		}

		// Get state from billing details
		state := "Unknown"
		if payment.BillingDetails != nil && payment.BillingDetails.Address != nil {
			state = payment.BillingDetails.Address.State
			if state == "" {
				state = "No State"
			}
		}

		// Create key for state + currency
		key := stateCurrencyKey{
			state:    state,
			currency: payment.Amount.Currency().Code,
		}

		// Aggregate by state and currency
		if existingTotal, ok := stateTotals[key]; ok {
			newTotal, addErr := existingTotal.Add(payment.Amount)
			if addErr != nil {
				log.Printf("Error adding amounts: %v", addErr)
				continue
			}
			stateTotals[key] = newTotal
		} else {
			stateTotals[key] = payment.Amount
		}
		stateCounts[key]++
	}

	// Print results
	fmt.Println("  Donation totals by state:")
	if len(stateTotals) == 0 {
		fmt.Println("    No donations found with state information")
		return
	}

	// Group by state for display
	stateGroups := make(map[string][]stateCurrencyKey)
	for key := range stateTotals {
		stateGroups[key.state] = append(stateGroups[key.state], key)
	}

	for state, keys := range stateGroups {
		for _, key := range keys {
			total := stateTotals[key]
			count := stateCounts[key]
			fmt.Printf("    %s (%s): %d donations, total %s\n",
				state, key.currency, count, total.Display())
		}
	}
}
