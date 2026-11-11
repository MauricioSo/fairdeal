package paypal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type SandboxClient struct {
	clientID     string
	clientSecret string
	baseURL      string
	httpClient   *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

func NewSandboxClient(clientID, clientSecret string) *SandboxClient {
	return &SandboxClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		baseURL:      "https://api-m.sandbox.paypal.com",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func (s *SandboxClient) getAccessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Return cached token if valid for at least another 60 seconds
	if s.accessToken != "" && time.Now().Add(60*time.Second).Before(s.expiresAt) {
		return s.accessToken, nil
	}

	endpoint := s.baseURL + "/v1/oauth2/token"
	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}

	basicAuth := base64.StdEncoding.EncodeToString([]byte(s.clientID + ":" + s.clientSecret))
	req.Header.Set("Authorization", "Basic "+basicAuth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("oauth token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("paypal oauth failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp oauthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode oauth response: %w", err)
	}

	s.accessToken = tokenResp.AccessToken
	s.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return s.accessToken, nil
}

type paypalCreateOrderResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (s *SandboxClient) CreateOrder(ctx context.Context, orderReq OrderRequest) (string, error) {
	token, err := s.getAccessToken(ctx)
	if err != nil {
		return "", err
	}

	bodyBytes, err := json.Marshal(orderReq)
	if err != nil {
		return "", err
	}

	endpoint := s.baseURL + "/v2/checkout/orders"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if len(orderReq.PurchaseUnits) > 0 {
		req.Header.Set("PayPal-Request-Id", orderReq.PurchaseUnits[0].ReferenceID)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create paypal order: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("paypal create order returned status %d", resp.StatusCode)
	}

	var res paypalCreateOrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("decode create order response: %w", err)
	}

	return res.ID, nil
}

func (s *SandboxClient) GetOrder(ctx context.Context, id string) (*Order, error) {
	token, err := s.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/v2/checkout/orders/%s", s.baseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get paypal order: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("paypal get order returned status %d", resp.StatusCode)
	}

	var order Order
	if err := json.NewDecoder(resp.Body).Decode(&order); err != nil {
		return nil, fmt.Errorf("decode get order response: %w", err)
	}

	return &order, nil
}

type paypalCaptureResponse struct {
	ID            string `json:"id"` // this is the ORDER id, not the capture id
	Status        string `json:"status"`
	PurchaseUnits []struct {
		Payments struct {
			Captures []struct {
				ID     string   `json:"id"`
				Status string   `json:"status"`
				Amount MoneyDTO `json:"amount"`
			} `json:"captures"`
		} `json:"payments"`
	} `json:"purchase_units"`
}

func (s *SandboxClient) CaptureOrder(ctx context.Context, id string, requestID string) (*Capture, error) {
	token, err := s.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/v2/checkout/orders/%s/capture", s.baseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if requestID != "" {
		req.Header.Set("PayPal-Request-Id", requestID)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("capture paypal order: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("paypal capture order returned status %d: %s", resp.StatusCode, string(body))
	}

	var res paypalCaptureResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode capture response: %w", err)
	}

	return parseCapture(&res)
}

// parseCapture extracts the real capture id/status/amount from PayPal's nested response.
// The order must be COMPLETED *and* contain a COMPLETED capture, otherwise it is not paid.
func parseCapture(res *paypalCaptureResponse) (*Capture, error) {
	if res.Status != "COMPLETED" || len(res.PurchaseUnits) == 0 || len(res.PurchaseUnits[0].Payments.Captures) == 0 {
		return &Capture{ID: res.ID, Status: res.Status}, nil
	}
	c := res.PurchaseUnits[0].Payments.Captures[0]
	return &Capture{ID: c.ID, Status: c.Status, Amount: c.Amount}, nil
}
