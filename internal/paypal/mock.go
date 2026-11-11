package paypal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
)

type mockOrderRecord struct {
	Order   Order
	Capture *Capture
}

type MockClient struct {
	mu     sync.Mutex
	orders map[string]*mockOrderRecord
}

func NewMockClient() *MockClient {
	return &MockClient{
		orders: make(map[string]*mockOrderRecord),
	}
}

func randHex(prefix string, bytesLen int) string {
	b := make([]byte, bytesLen)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func (m *MockClient) CreateOrder(ctx context.Context, req OrderRequest) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	orderID := randHex("MOCK-ORD-", 8)
	order := Order{
		ID:            orderID,
		Status:        "APPROVED", // Mock assumes approval for seamless demo workflow
		PurchaseUnits: req.PurchaseUnits,
	}

	m.orders[orderID] = &mockOrderRecord{
		Order: order,
	}

	return orderID, nil
}

func (m *MockClient) GetOrder(ctx context.Context, id string) (*Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rec, exists := m.orders[id]
	if !exists {
		return nil, ErrOrderNotFound
	}
	return &rec.Order, nil
}

func (m *MockClient) CaptureOrder(ctx context.Context, id string, requestID string) (*Capture, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rec, exists := m.orders[id]
	if !exists {
		return nil, ErrOrderNotFound
	}

	// Idempotency check: if already captured, return the existing capture
	if rec.Capture != nil {
		return rec.Capture, nil
	}

	val := "94.00"
	if len(rec.Order.PurchaseUnits) > 0 {
		val = rec.Order.PurchaseUnits[0].Amount.Value
	}

	captureID := randHex("MOCK-CAP-", 8)
	capResult := &Capture{
		ID:     captureID,
		Status: "COMPLETED",
		Amount: MoneyDTO{
			CurrencyCode: "USD",
			Value:        val,
		},
	}

	rec.Order.Status = "COMPLETED"
	rec.Capture = capResult

	return capResult, nil
}
