package clearing

import (
	"testing"
)

func TestV2ContractBindsMerchantProductPolicyAndDelivery(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	payload := AgreementTokenPayload{MerchantID: "alpine", SKU: "PARKA-M", PolicyID: 1, DeliveryCountry: "US", AgreementID: "agr-test", PriceCents: 10000, Shipping: ShippingExpress, ExpiresAtUnix: 100}
	token := Sign(payload, key)
	if Verify(payload, token, key, 90) != nil {
		t.Fatal("valid v2 token rejected")
	}
	for _, change := range []func(*AgreementTokenPayload){func(p *AgreementTokenPayload) { p.MerchantID = "nordic" }, func(p *AgreementTokenPayload) { p.SKU = "OTHER" }, func(p *AgreementTokenPayload) { p.PolicyID = 2 }, func(p *AgreementTokenPayload) { p.DeliveryCountry = "CL" }} {
		copy := payload
		change(&copy)
		if Verify(copy, token, key, 90) == nil {
			t.Fatal("tampered v2 contract accepted")
		}
	}
}
