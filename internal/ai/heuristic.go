package ai

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nashpay/nashpay/internal/store"
)

const num = `([0-9]+(?:\.[0-9]{1,2})?)`

var (
	// A budget is preferred when it follows a keyword, then when it carries a "$", then a currency word.
	budgetKeywordRe  = regexp.MustCompile(`(?i)(?:max(?:imum)?|budget|up\s*to|hasta|m[áa]ximo|under|below|at\s*most)\D{0,12}?` + num)
	budgetDollarRe   = regexp.MustCompile(`\$\s*` + num)
	budgetCurrencyRe = regexp.MustCompile(`(?i)` + num + `\s*(?:usd|dollars?|bucks|d[óo]lares)`)

	flexSlowRe = regexp.MustCompile(`(?i)(no\s*rush|no\s*hurry|not\s*in\s*a\s*hurry|not\s*in\s*a\s*rush|whenever|sin\s*apuro|no\s*tengo\s*prisa)`)
	flexFastRe = regexp.MustCompile(`(?i)(asap|urgent|tomorrow|fast|urgente|mañana)`)
	bnplRe     = regexp.MustCompile(`(?i)(installment|pay\s*in\s*4|pay\s*later|cuotas)`)
	bundleRe   = regexp.MustCompile(`(?i)(bundle|extra|gift|accessor|regalo)`)

	marginBeforeRe   = regexp.MustCompile(`(?i)` + num + `\s*%\s*(?:net\s*)?(?:margin|margen)`)
	marginAfterRe    = regexp.MustCompile(`(?i)(?:margin|margen)\D{0,12}?` + num)
	discountBeforeRe = regexp.MustCompile(`(?i)` + num + `\s*%\s*(?:max(?:imum)?\s*)?(?:discount|descuento)`)
	discountAfterRe  = regexp.MustCompile(`(?i)(?:discount|descuento)\D{0,15}?` + num)
	noBNPLRe         = regexp.MustCompile(`(?i)(?:no|disable|without|sin)\s*(?:pay\s*in\s*4|pay\s*later|cuotas)`)
	noSlowRe         = regexp.MustCompile(`(?i)(no\s*slow\s*shipping|express\s*only|solo\s*express)`)
)

func firstFloat(re *regexp.Regexp, text string) (float64, bool) {
	m := re.FindStringSubmatch(text)
	if len(m) < 2 {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	return v, err == nil
}

// ParseIntentHeuristic extracts buyer constraints via pattern matching without external calls.
// Defaults (spec §4.6): budget 100.00 when no amount is found, flexibility 0.50.
func ParseIntentHeuristic(text string) (budget float64, flex float64, bnpl bool, bundle bool) {
	budget = 100.00
	for _, re := range []*regexp.Regexp{budgetKeywordRe, budgetDollarRe, budgetCurrencyRe} {
		if v, ok := firstFloat(re, text); ok && v >= 1.0 {
			budget = v
			break
		}
	}
	if budget > 10000 {
		budget = 10000
	}

	flex = 0.50
	switch {
	case flexSlowRe.MatchString(text):
		flex = 0.85
	case flexFastRe.MatchString(text):
		flex = 0.10
	}

	return budget, flex, bnplRe.MatchString(text), bundleRe.MatchString(text)
}

// toFraction turns "20" or "0.2" into 0.20.
func toFraction(v float64) float64 {
	if v >= 1.0 {
		return v / 100.0
	}
	return v
}

// CompilePolicyHeuristic applies conservative edits from prompt text without external calls.
// It starts from the active policy and only changes what it recognises; the returned notes
// describe those edits. Guardrail clamping happens afterwards in CompileMerchantPolicy.
func CompilePolicyHeuristic(text string, cur store.PolicyConfig) (store.PolicyConfig, []string) {
	res := cur
	var notes []string

	if v, ok := firstFloat(marginBeforeRe, text); ok {
		res.MinMarginPct = toFraction(v)
		notes = append(notes, fmt.Sprintf("min margin %.0f%%", res.MinMarginPct*100))
	} else if v, ok := firstFloat(marginAfterRe, text); ok {
		res.MinMarginPct = toFraction(v)
		notes = append(notes, fmt.Sprintf("min margin %.0f%%", res.MinMarginPct*100))
	}

	if v, ok := firstFloat(discountBeforeRe, text); ok {
		res.MaxDiscountPct = toFraction(v)
		notes = append(notes, fmt.Sprintf("max discount %.0f%%", res.MaxDiscountPct*100))
	} else if v, ok := firstFloat(discountAfterRe, text); ok {
		res.MaxDiscountPct = toFraction(v)
		notes = append(notes, fmt.Sprintf("max discount %.0f%%", res.MaxDiscountPct*100))
	}

	if noSlowRe.MatchString(text) {
		res.AllowSlowShipping = false
		notes = append(notes, "slow shipping disabled")
	}
	if noBNPLRe.MatchString(text) {
		res.PayLaterEnabled = false
		notes = append(notes, "Pay Later disabled")
	}

	if len(notes) == 0 {
		res.Rationale = "No recognisable directives; policy unchanged (heuristic mode)"
	} else {
		res.Rationale = "Heuristic edits: " + strings.Join(notes, ", ")
	}
	return res, notes
}
