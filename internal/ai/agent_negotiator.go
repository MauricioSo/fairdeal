package ai

import (
	"fmt"
	"github.com/nashpay/nashpay/internal/negotiation"
)

type DialogueTurn struct {
	Turn    int    `json:"turn"`
	Speaker string `json:"speaker"`
	Role    string `json:"role"`
	Message string `json:"message"`
	Intent  string `json:"intent"`
}
type AgentNegotiationResult struct {
	Dialogue      []DialogueTurn `json:"dialogue"`
	AgreedPrice   string         `json:"agreed_price"`
	PaymentPlan   string         `json:"payment_plan"`
	IncludedGift  string         `json:"included_gift"`
	ShippingTier  string         `json:"shipping_tier"`
	SummaryReason string         `json:"summary_reason"`
	ModelUsed     string         `json:"model_used"`
}

// ExplainAgreement is a factual decision trace, not a fabricated agent conversation.
// AI is used upstream to parse mandates and compile merchant policies.
func ExplainAgreement(a negotiation.AgreementDetailsDTO, budget string) *AgentNegotiationResult {
	payment := "PayPal"
	if a.Concessions.PaymentMethod == "PAY_LATER" {
		payment = "PayPal Pay Later requested; availability and terms determined by PayPal"
	}
	gift := "No accessory included"
	if a.Concessions.Bundle != nil {
		gift = a.Concessions.Bundle.Name
	}
	return &AgentNegotiationResult{
		AgreedPrice: a.FinalPrice, PaymentPlan: payment, IncludedGift: gift, ShippingTier: a.Concessions.ShippingTier,
		SummaryReason: "The deterministic solver selected a feasible package. You decide whether to buy.", ModelUsed: "deterministic solver · decision trace",
		Dialogue: []DialogueTurn{
			{Turn: 1, Speaker: "Buyer mandate", Role: "buyer", Message: fmt.Sprintf("Maximum submitted budget: $%s. No automatic budget increase.", budget)},
			{Turn: 2, Speaker: "Seller constraints", Role: "seller", Message: "Validated configured margin floor, discount cap, product and accessory costs, shipping and estimated payment fees."},
			{Turn: 3, Speaker: "Validated recommendation", Role: "buyer", Message: fmt.Sprintf("Offer $%s; catalog checkout savings $%s; %s. %s.", a.FinalPrice, a.SavingsVsStandard, gift, payment)},
		},
	}
}
