package paypal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// VerifyWebhook delegates cryptographic validation to PayPal's verification API.
// The certificate URL is never fetched by this server.
func (s *SandboxClient) VerifyWebhook(ctx context.Context, headers http.Header, event json.RawMessage, webhookID string) (bool, error) {
	if webhookID == "" {
		return false, errors.New("webhook is not configured")
	}
	cert, err := url.Parse(headers.Get("Paypal-Cert-Url"))
	if err != nil || cert.Scheme != "https" || !(cert.Hostname() == "paypal.com" || strings.HasSuffix(cert.Hostname(), ".paypal.com")) {
		return false, nil
	}
	payload := map[string]any{"auth_algo": headers.Get("Paypal-Auth-Algo"), "cert_url": cert.String(), "transmission_id": headers.Get("Paypal-Transmission-Id"), "transmission_sig": headers.Get("Paypal-Transmission-Sig"), "transmission_time": headers.Get("Paypal-Transmission-Time"), "webhook_id": webhookID, "webhook_event": event}
	for _, key := range []string{"auth_algo", "transmission_id", "transmission_sig", "transmission_time"} {
		value := payload[key].(string)
		if value == "" || len(value) > 500 {
			return false, nil
		}
	}
	token, err := s.getAccessToken(ctx)
	if err != nil {
		return false, err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	request, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/v1/notifications/verify-webhook-signature", bytes.NewReader(data))
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.httpClient.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return false, errors.New("webhook verification unavailable")
	}
	var result struct {
		Status string `json:"verification_status"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&result); err != nil {
		return false, err
	}
	return result.Status == "SUCCESS", nil
}
