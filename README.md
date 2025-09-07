# Payments

A Go library for handling payment processing, currently with Stripe integration, developed for the International Combat Archery Alliance.

## Features

- **Generic Payment Interface**: Extensible design allowing for multiple payment providers
- **Stripe Integration**: Complete implementation for Stripe checkout sessions and webhook verification
- **Type-Safe Money Handling**: Uses `go-money` library for precise currency handling
- **Webhook Validation**: Secure webhook signature verification
- **Comprehensive Error Handling**: Structured error types with detailed context

## Installation

```bash
go get github.com/International-Combat-Archery-Alliance/payments
```

## Usage

### Basic Setup

```go
import (
    "github.com/International-Combat-Archery-Alliance/payments"
    "github.com/International-Combat-Archery-Alliance/payments/stripe"
    "github.com/Rhymond/go-money"
)

// Initialize Stripe client
client := stripe.NewClient("sk_test_...", "whsec_...")
```

### Creating a Checkout Session

```go
params := payments.CheckoutParams{
    ReturnURL: "https://yoursite.com/success",
    Items: []payments.Item{
        {
            Name:  "Tournament Registration",
            Price: money.New(2500, money.USD), // $25.00
        },
    },
    Metadata: map[string]string{
        "user_id": "12345",
        "event_id": "tournament_2024",
    },
    AllowAdaptivePricing: true,
}

checkoutInfo, err := client.CreateCheckout(ctx, params)
if err != nil {
    // Handle error
}

// Use checkoutInfo.ClientSecret for frontend integration
```

### Handling Webhooks

```go
func handleWebhook(w http.ResponseWriter, r *http.Request) {
    payload, _ := ioutil.ReadAll(r.Body)
    signature := r.Header.Get("Stripe-Signature")
    
    metadata, err := client.ConfirmCheckout(ctx, payload, signature)
    if err != nil {
        // Handle webhook verification error
        return
    }
    
    // Process successful payment using metadata
}
```

## Error Handling

The library provides structured error types with specific reasons:

- `ErrorReasonSignatureValidation`: Webhook signature validation failed
- `ErrorReasonFailedToCreateCheckoutSession`: Checkout session creation failed
- `ErrorReasonInvalidWebhookEventData`: Invalid webhook event data
- `ErrorReasonNotCheckoutConfirmedEvent`: Event is not a checkout completion
- `ErrorReasonNotPaid`: Payment status is not paid

## Dependencies

- [Stripe Go SDK](https://github.com/stripe/stripe-go) - Stripe API integration
- [go-money](https://github.com/Rhymond/go-money) - Currency handling

## License

Licensed under the terms specified in the LICENSE file.
