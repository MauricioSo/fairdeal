package ai

import (
	"context"
	"fmt"
	"strings"
)

type BuyerConstraintsOutput struct {
	Budget              string  `json:"budget"`
	ShippingFlexibility float64 `json:"shipping_flexibility"`
	PrefersPayLater     bool    `json:"prefers_pay_later"`
	WillingToBundle     bool    `json:"willing_to_bundle"`
}

type DiscoveredProductDTO struct {
	SKU         string   `json:"sku"`
	Name        string   `json:"name"`
	MatchReason string   `json:"match_reason"`
	Features    []string `json:"features"`
}

type DynamicBundleOption struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
	Active bool   `json:"active"`
}

type ParseIntentResponse struct {
	DiscoveredProduct DiscoveredProductDTO   `json:"discovered_product"`
	Constraints       BuyerConstraintsOutput `json:"constraints"`
	SuggestedBundles  []DynamicBundleOption  `json:"suggested_bundles"`
	AgentAdvice       string                 `json:"agent_advice"`
	Source            string                 `json:"source"` // "llm" or "heuristic"
	Notes             string                 `json:"notes"`
}

type llmIntentOutput struct {
	ProductMatch        DiscoveredProductDTO  `json:"product_match"`
	BudgetUSD           float64               `json:"budget_usd"`
	ShippingFlexibility float64               `json:"shipping_flexibility"`
	PrefersPayLater     bool                  `json:"prefers_pay_later"`
	WillingToBundle     bool                  `json:"willing_to_bundle"`
	SuggestedBundles    []DynamicBundleOption `json:"suggested_bundles"`
	AgentAdvice         string                `json:"agent_advice"`
	Notes               string                `json:"notes"`
}

const buyerIntentSystemPrompt = `You are an autonomous Buyer Agent in Fairdeal.
Your job is twofold:
1. Product Discovery: Identify the catalog product that best matches the buyer's requested characteristics. (Catalog default: SKU "CH-WINTER-PARKA-01", "Winter Parka Performance 01", 3-layer weatherproof membrane, 800-fill hydrophobic down, list $120.00 + $15 express shipping = $135.00 total).
2. Construct Negotiation Levers: Synthesize dynamic, personalized negotiation dimensions based on the buyer's prompt:
   - Target Budget (extract from text or recommend fair market value ~$95-$110).
   - Delivery Urgency / Shipping Flexibility (0.0 = ASAP 1-2d express, 1.0 = flexible ground 5-7d saving $11 in freight).
   - Financing preference (true if mentions installments, Pay in 4, or cashflow ease).
   - Only stocked accessory: SKU-SCARF-09 Cashmere Scarf ($25 catalog value). Do not invent gloves, hats, products or sellers.
   - Strategic advice from the Buyer Agent explaining how these levers will be used with the seller network.

Return ONLY valid JSON matching this exact schema:
{
  "product_match": {
    "sku": "CH-WINTER-PARKA-01",
    "name": "Winter Parka Performance 01",
    "match_reason": "string explaining how it matches user requirements",
    "features": ["feature 1", "feature 2", "feature 3"]
  },
  "budget_usd": number,
  "shipping_flexibility": number (0.0 to 1.0),
  "prefers_pay_later": boolean,
  "willing_to_bundle": boolean,
  "suggested_bundles": [
    {
      "id": "scarf",
      "name": "Cashmere Winter Scarf ($25 value)",
      "reason": "why this bundle is relevant to user",
      "active": true
    },
    {
      "id": "gloves",
      "name": "Thermal Pro Ski Gloves ($35 value)",
      "reason": "why this bundle is relevant to user",
      "active": true
    }
  ],
  "agent_advice": "strategic recommendation explaining how to negotiate (in English)",
  "notes": "brief summary (in English)"
}
Language rule: ALWAYS output all JSON string values (match_reason, features, reason, agent_advice, notes) in English, even if the user's input prompt is written in Spanish or another language.`

// ParseBuyerIntent parses natural language preferences into structured constraints.
func ParseBuyerIntent(ctx context.Context, text string, client *LLMClient) ParseIntentResponse {
	text = strings.TrimSpace(text)
	if text == "" {
		text = "I have a budget of $105 and no rush on shipping."
	}

	if client != nil && client.IsConfigured() {
		var out llmIntentOutput
		err := client.ChatJSON(ctx, buyerIntentSystemPrompt, text, &out)
		if err == nil {
			// Clamp values
			b := out.BudgetUSD
			if b < 1.0 {
				b = 1.0
			}
			if b > 10000.0 {
				b = 10000.0
			}

			flex := out.ShippingFlexibility
			if flex < 0.0 {
				flex = 0.0
			}
			if flex > 1.0 {
				flex = 1.0
			}

			out.ProductMatch.SKU = "CH-WINTER-PARKA-01"
			out.ProductMatch.Name = "Winter Parka Performance 01"
			disc := out.ProductMatch
			if disc.SKU == "" {
				disc = DiscoveredProductDTO{
					SKU:         "CH-WINTER-PARKA-01",
					Name:        "Winter Parka Performance 01",
					MatchReason: "Engineered 3-layer weatherproof membrane with 800-fill down for extreme cold & snow.",
					Features:    []string{"800-fill hydrophobic down", "3-layer weatherproof membrane", "Thermal draft collars"},
				}
			}

			bundles := []DynamicBundleOption{{ID: "scarf", Name: "Cashmere Scarf ($25 catalog value)", Reason: "The stocked demo accessory", Active: out.WillingToBundle}}
			if len(bundles) == 0 {
				bundles = []DynamicBundleOption{
					{ID: "scarf", Name: "Cashmere Winter Scarf ($25 value)", Reason: "Surplus accessory matching cold-weather mandate", Active: true},
				}
			}

			advice := out.AgentAdvice
			if advice == "" {
				advice = fmt.Sprintf("I matched the Winter Parka Performance 01 to your requirements and set your target budget to $%s. Adjust the levers below before we query the sellers.", fmt.Sprintf("%.2f", b))
			}

			return ParseIntentResponse{
				DiscoveredProduct: disc,
				Constraints: BuyerConstraintsOutput{
					Budget:              fmt.Sprintf("%.2f", b),
					ShippingFlexibility: flex,
					PrefersPayLater:     out.PrefersPayLater,
					WillingToBundle:     out.WillingToBundle,
				},
				SuggestedBundles: bundles,
				AgentAdvice:      advice,
				Source:           "llm",
				Notes:            out.Notes,
			}
		}
	}

	// Heuristic fallback
	budget, flex, bnpl, bundle := ParseIntentHeuristic(text)
	return ParseIntentResponse{
		DiscoveredProduct: DiscoveredProductDTO{
			SKU:         "CH-WINTER-PARKA-01",
			Name:        "Winter Parka Performance 01",
			MatchReason: "Engineered extreme-weather technical parka for sub-zero snow and outdoor activities.",
			Features:    []string{"800-fill hydrophobic down", "3-layer weatherproof membrane", "Thermal draft collars"},
		},
		Constraints: BuyerConstraintsOutput{
			Budget:              fmt.Sprintf("%.2f", budget),
			ShippingFlexibility: flex,
			PrefersPayLater:     bnpl,
			WillingToBundle:     bundle,
		},
		SuggestedBundles: []DynamicBundleOption{
			{ID: "scarf", Name: "Cashmere Winter Scarf ($25 value)", Reason: "Liquidates warehouse overstock while keeping you warm", Active: true},
		},
		AgentAdvice: fmt.Sprintf("Buyer Agent: Configured target budget at $%.2f. Delivery flexibility is set to %.0f%% to unlock freight credits.", budget, flex*100),
		Source:      "heuristic",
		Notes:       "Parsed using pattern rules engine",
	}
}
