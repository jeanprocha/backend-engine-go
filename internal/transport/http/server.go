package http

import (
	"context"
	"net/http"

	"github.com/jeanprocha/backend-engine-go/internal/auth"
	"github.com/jeanprocha/backend-engine-go/internal/classifier"
	"github.com/jeanprocha/backend-engine-go/internal/company"
	"github.com/jeanprocha/backend-engine-go/internal/history"
	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
	"github.com/jeanprocha/backend-engine-go/internal/plg"
	"github.com/jeanprocha/backend-engine-go/internal/rag"
	"github.com/jeanprocha/backend-engine-go/internal/strategytags"
	"github.com/jeanprocha/backend-engine-go/internal/tax"
)

// AuthRouteConfig controla proteção das rotas de utilizador (histórico e empresas).
type AuthRouteConfig struct {
	// DevSkip: se true, usa header X-User-ID (apenas desenvolvimento; defina AUTH_SKIP=true).
	DevSkip bool
	// Verifier: validação JWT Clerk; obrigatório quando DevSkip é false.
	Verifier *auth.ClerkVerifier
	// Plg: limites por plano (simulações/dia Free, empresas). nil = desligado.
	Plg *plg.Limiter
}

// Server encapsula o http.Server e as dependências necessárias pelos handlers.
type Server struct {
	httpServer        *http.Server
	store             *ingestion.Store
	rag               *rag.Service
	tax               tax.Engine
	classifier        *classifier.Service
	history           *history.Repo
	companies         *company.Repo
	strategyTagsRepo  *strategytags.Repo
	strategyTagsCache *strategytags.ListCache
	plg               *plg.Limiter
	clerkVerifier     *auth.ClerkVerifier
	authDevSkip       bool
	// generateDiagnosticPDF gera o PDF de diagnóstico a partir do histórico (nil = rota desativada).
	generateDiagnosticPDF func(*history.Detail) ([]byte, error)
}

// NewServer cria e configura o servidor com todas as rotas e middlewares.
// addr: ex. "0.0.0.0:8080" (Railway PORT) — ver internal/config.ListenAddr.
func NewServer(addr string, store *ingestion.Store, ragSvc *rag.Service, taxEngine tax.Engine, classifierSvc *classifier.Service, hist *history.Repo, compRepo *company.Repo, tagRepo *strategytags.Repo, tagCache *strategytags.ListCache, authCfg AuthRouteConfig, diagnosticPDF func(*history.Detail) ([]byte, error)) *Server {
	s := &Server{
		store:                 store,
		rag:                   ragSvc,
		tax:                   taxEngine,
		classifier:            classifierSvc,
		history:               hist,
		companies:             compRepo,
		strategyTagsRepo:      tagRepo,
		strategyTagsCache:     tagCache,
		plg:                   authCfg.Plg,
		clerkVerifier:         authCfg.Verifier,
		authDevSkip:           authCfg.DevSkip,
		generateDiagnosticPDF: diagnosticPDF,
	}

	rl := newRateLimiter()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.healthHandler)
	mux.HandleFunc("GET /ready", s.readyHandler)
	mux.Handle("GET /strategy-tags", rl.Wrap(http.HandlerFunc(s.strategyTagsHandler)))
	mux.Handle("POST /ai/explanations", rl.Wrap(http.HandlerFunc(s.ragHandler)))
	mux.Handle("POST /simulations", rl.Wrap(http.HandlerFunc(s.simulationHandler)))
	mux.Handle("POST /credit-classifications", rl.Wrap(http.HandlerFunc(s.classificationHandler)))
	mux.Handle("POST /credit-classifications/batch", rl.Wrap(http.HandlerFunc(s.classificationBatchHandler)))
	mux.Handle("GET /law/corpus", rl.Wrap(http.HandlerFunc(s.lawCorpusHandler)))
	mux.Handle("GET /engine/validation", rl.Wrap(http.HandlerFunc(s.engineValidationHandler)))
	mux.Handle("GET /law/articles/{id}", rl.Wrap(http.HandlerFunc(s.lawArticleHandler)))
	mux.Handle("GET /law/articles/{id}/pdf-anchor", rl.Wrap(protectRoute(authCfg.DevSkip, authCfg.Verifier, http.HandlerFunc(s.lawPdfAnchorHandler))))
	mux.Handle("POST /simulation-records", protectRoute(authCfg.DevSkip, authCfg.Verifier, http.HandlerFunc(s.saveSimulationRecordHandler)))
	mux.Handle("GET /simulation-records", protectRoute(authCfg.DevSkip, authCfg.Verifier, http.HandlerFunc(s.listSimulationRecordsHandler)))
	mux.Handle("GET /simulation-records/{id}/report", protectRoute(authCfg.DevSkip, authCfg.Verifier, http.HandlerFunc(s.simulationRecordReportHandler)))
	mux.Handle("GET /simulation-records/{id}", protectRoute(authCfg.DevSkip, authCfg.Verifier, http.HandlerFunc(s.getSimulationRecordHandler)))
	// Dossié partilhável: leitura pública; o UUID de simulação funciona como segredo de partilha.
	// Rate limit (Etapa M/PR 11): o link /exemplo passa a ser divulgado na landing —
	// mesmo limitador por IP das demais rotas públicas, não um orçamento próprio.
	mux.Handle("GET /public/simulation-records/{id}", rl.Wrap(http.HandlerFunc(s.getPublicSimulationRecordHandler)))
	mux.Handle("GET /companies", protectRoute(authCfg.DevSkip, authCfg.Verifier, http.HandlerFunc(s.listCompaniesHandler)))
	mux.Handle("POST /companies", protectRoute(authCfg.DevSkip, authCfg.Verifier, http.HandlerFunc(s.createCompanyHandler)))
	mux.Handle("DELETE /companies/{id}", protectRoute(authCfg.DevSkip, authCfg.Verifier, http.HandlerFunc(s.deleteCompanyHandler)))
	mux.Handle("GET /plg/quota", protectRoute(authCfg.DevSkip, authCfg.Verifier, http.HandlerFunc(s.plgQuotaHandler)))

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: chain(mux, withRequestID, withLogger, withCORS),
	}

	return s
}

// Start inicia o servidor HTTP. Bloqueia até que o servidor seja encerrado.
// Retorna http.ErrServerClosed quando Shutdown for chamado normalmente.
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown encerra o servidor de forma limpa, aguardando as conexões em andamento.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
