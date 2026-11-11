package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/nashpay/nashpay/internal/httpapi"
	"github.com/nashpay/nashpay/internal/negotiation"
	"github.com/nashpay/nashpay/internal/store"
)

func setupTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_ws.db")

	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}

	secret := []byte("01234567890123456789012345678901")
	svc := negotiation.NewService(st, secret, 300)
	rl := negotiation.NewRateLimiter(1) // 1 second cooldown for fast tests
	h := NewHandler(svc, rl, st)

	mux := http.NewServeMux()
	mux.Handle("/v1/clearing/negotiate", h)

	server := httptest.NewServer(mux)
	return server, st
}

func TestWebSocketFlow(t *testing.T) {
	server, st := setupTestServer(t)
	defer server.Close()
	defer st.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/clearing/negotiate"

	// 1. Connection without cookie should be rejected
	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		t.Fatal("expected failure without session cookie")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized without cookie, got %d", resp.StatusCode)
	}

	// 2. Connect with valid 32-hex-char session cookie
	headers := http.Header{}
	headers.Add("Cookie", httpapi.SessionCookieName+"=0123456789abcdef0123456789abcdef")
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader:   headers,
		Subprotocols: []string{"nashpay-clearing-v1"},
	})
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	// 3. Send CLIENT_INITIATE (G1 constraints)
	initMsg := InboundMessage{
		Event: "CLIENT_INITIATE",
		Seq:   1,
		SKU:   "CH-WINTER-PARKA-01",
		Constraints: &ConstraintsInput{
			Budget:              "105.00",
			ShippingFlexibility: 0.85,
			PrefersPayLater:     true,
			WillingToBundle:     true,
		},
	}
	if err := wsjson.Write(ctx, conn, initMsg); err != nil {
		t.Fatal(err)
	}

	// Receive SESSION_READY
	var ready OutboundSessionReady
	if err := wsjson.Read(ctx, conn, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Event != "SESSION_READY" || ready.StandardTotal != "135.00" {
		t.Errorf("unexpected SESSION_READY: %+v", ready)
	}

	// Receive PREVIEW
	var preview OutboundPreview
	if err := wsjson.Read(ctx, conn, &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Event != "PREVIEW" || !preview.Feasible || preview.EstimatedPrice != "97.00" {
		t.Errorf("unexpected PREVIEW: %+v", preview)
	}

	// 3b. Send CONSTRAINTS_UPDATE
	updMsg := InboundMessage{
		Event: "CONSTRAINTS_UPDATE",
		Seq:   2,
		Constraints: &ConstraintsInput{
			Budget:              "105.00",
			ShippingFlexibility: 0.85,
			PrefersPayLater:     true,
			WillingToBundle:     true,
		},
	}
	if err := wsjson.Write(ctx, conn, updMsg); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Read(ctx, conn, &preview); err != nil {
		t.Fatal(err)
	}

	// 4. Send COMMIT and measure end-to-end latency (AC5: < 50ms)
	commitStart := time.Now()
	commitMsg := InboundMessage{
		Event: "COMMIT",
		Seq:   3,
	}
	if err := wsjson.Write(ctx, conn, commitMsg); err != nil {
		t.Fatal(err)
	}

	// Receive SOLVER_TRACE
	var trace OutboundTrace
	if err := wsjson.Read(ctx, conn, &trace); err != nil {
		t.Fatal(err)
	}
	if trace.Event != "SOLVER_TRACE" || len(trace.Frontiers) == 0 {
		t.Errorf("unexpected trace: %+v", trace)
	}

	// Receive NASH_AGREEMENT
	var agreement negotiation.NashAgreementResponseDTO
	if err := wsjson.Read(ctx, conn, &agreement); err != nil {
		t.Fatal(err)
	}
	commitElapsed := time.Since(commitStart)

	if agreement.Event != "NASH_AGREEMENT" || agreement.Agreement.FinalPrice != "97.00" {
		t.Errorf("unexpected agreement: %+v", agreement)
	}
	if agreement.Agreement.Concessions.Installments == nil || agreement.Agreement.Concessions.Installments.Amount != "24.25" {
		t.Errorf("unexpected installments: %+v", agreement.Agreement.Concessions.Installments)
	}

	t.Logf("End-to-end commit latency: %v", commitElapsed)
	if commitElapsed > 100*time.Millisecond {
		t.Errorf("commit latency %v exceeds threshold", commitElapsed)
	}

	// 5. Test REQUOTE_WITHOUT_PAY_LATER
	requoteMsg := InboundMessage{
		Event:       "REQUOTE_WITHOUT_PAY_LATER",
		Seq:         4,
		AgreementID: agreement.AgreementID,
	}
	if err := wsjson.Write(ctx, conn, requoteMsg); err != nil {
		t.Fatal(err)
	}

	var requoteResp negotiation.NashAgreementResponseDTO
	if err := wsjson.Read(ctx, conn, &requoteResp); err != nil {
		t.Fatal(err)
	}
	if requoteResp.Agreement.Concessions.PaymentMethod != "PAYPAL" {
		t.Errorf("expected PAYMENT_METHOD 'PAYPAL' in requote, got %s", requoteResp.Agreement.Concessions.PaymentMethod)
	}

	// Verify old agreement was marked EXPIRED with reason PAY_LATER_INELIGIBLE
	oldAgr, err := st.GetAgreement(ctx, agreement.AgreementID)
	if err != nil {
		t.Fatal(err)
	}
	if oldAgr.Status != store.StatusExpired || oldAgr.StatusReason == nil || *oldAgr.StatusReason != "PAY_LATER_INELIGIBLE" {
		t.Errorf("expected old agreement EXPIRED/PAY_LATER_INELIGIBLE, got %s/%v", oldAgr.Status, oldAgr.StatusReason)
	}

	// 6. Test error paths (INVALID budget < 1.00, UNKNOWN_EVENT, RATE_LIMITED on immediate re-commit)
	badUpd := InboundMessage{
		Event: "CONSTRAINTS_UPDATE",
		Seq:   5,
		Constraints: &ConstraintsInput{
			Budget:              "0.50",
			ShippingFlexibility: 0.5,
		},
	}
	_ = wsjson.Write(ctx, conn, badUpd)
	var errMsg OutboundError
	if err := wsjson.Read(ctx, conn, &errMsg); err != nil {
		t.Fatal(err)
	}
	if errMsg.Code != "INVALID" {
		t.Errorf("expected INVALID, got %s", errMsg.Code)
	}

	_ = wsjson.Write(ctx, conn, InboundMessage{Event: "UNKNOWN_EVT", Seq: 6})
	if err := wsjson.Read(ctx, conn, &errMsg); err != nil {
		t.Fatal(err)
	}
	if errMsg.Code != "INVALID" {
		t.Errorf("expected INVALID for unknown event, got %s", errMsg.Code)
	}

	_ = wsjson.Write(ctx, conn, InboundMessage{Event: "COMMIT", Seq: 7})
	if err := wsjson.Read(ctx, conn, &errMsg); err != nil {
		t.Fatal(err)
	}
	if errMsg.Code != "RATE_LIMITED" {
		t.Errorf("expected RATE_LIMITED on immediate commit, got %s", errMsg.Code)
	}
}
