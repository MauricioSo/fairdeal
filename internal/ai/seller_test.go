package ai

import (
	"context"
	"encoding/json"
	"github.com/nashpay/nashpay/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSellerAgentReceivesOnlyPublicFieldsAndCannotInventPermissions(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload chatRequest
		if e := json.NewDecoder(r.Body).Decode(&payload); e != nil {
			t.Fatal(e)
		}
		for _, message := range payload.Messages {
			if message.Role == "user" {
				for _, key := range []string{"budget", "buyer_prompt", "buyer_surplus", "competitor"} {
					if strings.Contains(message.Content, key) {
						t.Errorf("private context: %s", key)
					}
				}
			}
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"include_accessory\":true,\"explore_pay_later\":true}"}}]}`))
	}))
	defer server.Close()
	proposal := ProposeSeller(context.Background(), NewLLMClient(server.URL, "test", "test"), SellerRequest{Merchant: "Alpine", Product: "Parka", Policy: store.PolicyConfig{}, Round: 1})
	if calls != 1 || proposal.IncludeAccessory || proposal.ExplorePayLater || proposal.Source != "llm" {
		t.Fatalf("unsafe seller proposal: %+v", proposal)
	}
}
