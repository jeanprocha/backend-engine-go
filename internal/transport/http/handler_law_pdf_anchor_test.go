package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// As duas portas para a MESMA resolução de ancoragem diferem só em quem entra.
// Um id vazio nunca chega ao store (a validação vem antes), então o status
// distingue exatamente o que se quer travar aqui: a rota autenticada barra
// primeiro por falta de sessão; a pública passa direto para a validação.
//
// Contexto: o dossiê compartilhável é lido por quem não tem conta, e pela rota
// autenticada o botão "Abrir no PDF oficial" respondia 401 — a prova documental
// morria justamente para o destinatário do parecer.

func TestLawPdfAnchor_RotaAutenticada_ExigeSessao(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/law/articles/lc214_0048_art_47_p1/pdf-anchor", nil)
	rec := httptest.NewRecorder()

	s.lawPdfAnchorHandler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sem sessão a rota autenticada deve devolver 401 (gate de plano intacto), obtido %d", rec.Code)
	}
}

func TestLawPdfAnchor_RotaPublica_NaoExigeSessao(t *testing.T) {
	s := &Server{}
	// PathValue vazio: o handler valida o id antes de tocar no store, então
	// 400 prova que passou do ponto onde a rota autenticada teria dado 401.
	req := httptest.NewRequest(http.MethodGet, "/public/law-articles//pdf-anchor", nil)
	rec := httptest.NewRecorder()

	s.publicLawPdfAnchorHandler(rec, req)

	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Fatalf("a rota pública não pode exigir sessão nem plano, obtido %d", rec.Code)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("id vazio deve devolver 400, obtido %d", rec.Code)
	}
}

func TestLawPdfAnchor_RotaPublica_RejeitaMetodoErrado(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/public/law-articles/lc214_0048_art_47_p1/pdf-anchor", nil)
	rec := httptest.NewRecorder()

	s.publicLawPdfAnchorHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("esperado 405, obtido %d", rec.Code)
	}
}
