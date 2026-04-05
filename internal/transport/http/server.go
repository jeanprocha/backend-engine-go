package http

import (
	"context"
	"net/http"

	"github.com/jeanprocha/backend-engine-go/internal/classifier"
	"github.com/jeanprocha/backend-engine-go/internal/history"
	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
	"github.com/jeanprocha/backend-engine-go/internal/rag"
	"github.com/jeanprocha/backend-engine-go/internal/tax"
)

// Server encapsula o http.Server e as dependências necessárias pelos handlers.
type Server struct {
	httpServer *http.Server
	store      *ingestion.Store
	rag        *rag.Service
	tax        tax.Engine
	classifier *classifier.Service
	history    *history.Repo
}

// NewServer cria e configura o servidor com todas as rotas e middlewares.
// addr deve ser no formato ":8080".
func NewServer(addr string, store *ingestion.Store, ragSvc *rag.Service, taxEngine tax.Engine, classifierSvc *classifier.Service, hist *history.Repo) *Server {
	s := &Server{store: store, rag: ragSvc, tax: taxEngine, classifier: classifierSvc, history: hist}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.healthHandler)
	mux.HandleFunc("POST /ai/explanations", s.ragHandler)
	mux.HandleFunc("POST /simulations", s.simulationHandler)
	mux.HandleFunc("POST /credit-classifications", s.classificationHandler)
	mux.HandleFunc("POST /credit-classifications/batch", s.classificationBatchHandler)
	mux.HandleFunc("POST /simulation-records", s.saveSimulationRecordHandler)
	mux.HandleFunc("GET /simulation-records", s.listSimulationRecordsHandler)
	mux.HandleFunc("GET /simulation-records/{id}", s.getSimulationRecordHandler)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: chain(mux, withLogger, withCORS),
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
