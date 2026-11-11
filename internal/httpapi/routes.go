package httpapi

import (
	"io/fs"
	"net/http"

	"github.com/nashpay/nashpay/internal/config"
	"github.com/nashpay/nashpay/internal/money"
	"github.com/nashpay/nashpay/internal/store"
	"github.com/nashpay/nashpay/web"
)

type ProductResponseDTO struct {
	SKU              string             `json:"sku"`
	Name             string             `json:"name"`
	ListPrice        string             `json:"list_price"`
	StandardShipping string             `json:"standard_shipping"`
	StandardTotal    string             `json:"standard_total"`
	Image            string             `json:"image"`
	BundleOffer      *BundleOfferDetail `json:"bundle_offer"`
}

type BundleOfferDetail struct {
	SKU         string `json:"sku"`
	Name        string `json:"name"`
	RetailValue string `json:"retail_value"`
}

type ConfigResponseDTO struct {
	PayPalMode     string `json:"paypal_mode"`
	PayPalClientID string `json:"paypal_client_id"`
	Currency       string `json:"currency"`
	AIMode         string `json:"ai_mode"`
	DemoMode       bool   `json:"demo_mode"`
}

type Server struct {
	Mux         *http.ServeMux
	Store       *store.Store
	Config      *config.Config
	WSHandler   http.Handler
	AIHandler   *AIHandler
	PPHandler   *PayPalHandler
	MchtHandler *MerchantHandler
}

func NewServer(st *store.Store, cfg *config.Config, wsHandler http.Handler, ppHandler *PayPalHandler, aiHandler *AIHandler, mchtHandler *MerchantHandler) *Server {
	s := &Server{
		Mux:         http.NewServeMux(),
		Store:       st,
		Config:      cfg,
		WSHandler:   wsHandler,
		PPHandler:   ppHandler,
		AIHandler:   aiHandler,
		MchtHandler: mchtHandler,
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.Mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	s.Mux.HandleFunc("GET /api/config", s.handleConfig)
	s.Mux.HandleFunc("GET /api/products/{sku}", s.handleProduct)

	if s.WSHandler != nil {
		s.Mux.Handle("GET /v1/clearing/negotiate", s.WSHandler)
	}
	if s.PPHandler != nil {
		s.PPHandler.Register(s.Mux)
	}
	if s.AIHandler != nil {
		s.AIHandler.Register(s.Mux)
	}
	if s.MchtHandler != nil {
		s.MchtHandler.Register(s.Mux)
	}
	s.registerStatic()
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	dto := ConfigResponseDTO{
		PayPalMode: s.Config.PayPalMode,
		Currency:   "USD",
		AIMode:     s.Config.AIMode,
		DemoMode:   s.Config.DemoMode,
	}
	if s.Config.PayPalMode == "sandbox" {
		dto.PayPalClientID = s.Config.PayPalClientID
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) handleProduct(w http.ResponseWriter, r *http.Request) {
	prod, err := s.Store.GetProduct(r.Context(), r.PathValue("sku"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	mkt, err := s.Store.GetMarketSettings(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	list := money.Cents(prod.ListPriceCents)
	ship := money.Cents(mkt.StandardShippingChargeCents)

	var offer *BundleOfferDetail
	if pol, err := s.Store.GetActivePolicy(r.Context()); err == nil && pol.Policy.BundleSKU != nil {
		if bp, err := s.Store.GetProduct(r.Context(), *pol.Policy.BundleSKU); err == nil {
			offer = &BundleOfferDetail{SKU: bp.SKU, Name: bp.Name, RetailValue: money.Cents(bp.RetailValueCents).Format()}
		}
	}

	writeJSON(w, http.StatusOK, ProductResponseDTO{
		SKU:              prod.SKU,
		Name:             prod.Name,
		ListPrice:        list.Format(),
		StandardShipping: ship.Format(),
		StandardTotal:    (list + ship).Format(),
		Image:            "/img/parka.jpg",
		BundleOffer:      offer,
	})
}

// registerStatic serves the embedded UI. Every HTML entry point issues the session cookie.
func (s *Server) registerStatic() {
	staticContent, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		return
	}
	fileServer := http.FileServer(http.FS(staticContent))
	serve := func(w http.ResponseWriter, r *http.Request) {
		_ = GetOrCreateSessionID(w, r)
		fileServer.ServeHTTP(w, r)
	}
	alias := func(page string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			r2 := r.Clone(r.Context())
			r2.URL.Path = page
			serve(w, r2)
		}
	}

	s.Mux.HandleFunc("GET /merchant", alias("/merchant.html"))
	s.Mux.HandleFunc("GET /checkout/confirmed", alias("/confirmed.html"))
	s.Mux.HandleFunc("GET /", serve)
}
