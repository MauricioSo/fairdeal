package kernel

import "context"

type Client struct{ baseURL, apiKey string }
type MarketVerification struct {
	Verified         bool   `json:"verified"`
	Status           string `json:"status"`
	BrowserSessionID string `json:"browser_session_id,omitempty"`
	LiveMSRP         string `json:"live_msrp,omitempty"`
	DealPrice        string `json:"deal_price,omitempty"`
	NetBuyerBenefit  string `json:"net_buyer_benefit,omitempty"`
	LatencyMs        int64  `json:"latency_ms,omitempty"`
	AgentModel       string `json:"agent_model,omitempty"`
	ProofTimestamp   string `json:"proof_timestamp,omitempty"`
}

func NewClient(baseURL, apiKey string) *Client { return &Client{baseURL: baseURL, apiKey: apiKey} }
func (c *Client) IsConfigured() bool           { return c != nil && c.apiKey != "" }
func (c *Client) CheckConnection(ctx context.Context) (string, bool) {
	return "Market verification unavailable: no product source and extraction configured", false
}
func (c *Client) VerifyMarketPrice(ctx context.Context, sku, dealPrice string) (*MarketVerification, error) {
	return &MarketVerification{Verified: false, Status: "Market price not verified. Savings compare the seeded catalog checkout, not live retail prices.", DealPrice: dealPrice}, nil
}
