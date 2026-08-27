package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestEngineValidationResponseJSON_EmptySlicesSerializeAsArrays mesma lição
// de TestLawCorpusResponseJSON_EmptySlicesSerializeAsArrays: um slice Go nil
// serializa "null" em JSON — o frontend faz data.cases.map(...)/scope.join(...)
// sem checar null.
func TestEngineValidationResponseJSON_EmptySlicesSerializeAsArrays(t *testing.T) {
	resp := EngineValidationResponse{
		Validated:  false,
		Reference:  EngineValidationReferenceResponse{},
		Scope:      []string{},
		OutOfScope: []string{},
		Cases:      []EngineValidationCaseResponse{},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, `"scope":[]`) {
		t.Errorf("scope deveria serializar como [], obteve: %s", s)
	}
	if !strings.Contains(s, `"out_of_scope":[]`) {
		t.Errorf("out_of_scope deveria serializar como [], obteve: %s", s)
	}
	if !strings.Contains(s, `"cases":[]`) {
		t.Errorf("cases deveria serializar como [], obteve: %s", s)
	}
	if strings.Contains(s, "null") {
		t.Errorf("nenhum campo deveria serializar null: %s", s)
	}
}

func TestEngineValidationResponseJSON_RoundTrip(t *testing.T) {
	resp := EngineValidationResponse{
		Validated: true,
		Reference: EngineValidationReferenceResponse{
			Name:    "Calculadora de Tributos RFB/Serpro",
			URL:     "http://localhost:8080/api",
			Version: "1.0.0-beta",
			RunAt:   "2026-08-27T12:00:00Z",
		},
		Scope:      []string{"CBS", "IBS"},
		OutOfScope: []string{"ICMS"},
		Tolerance:  "0.01",
		Cases: []EngineValidationCaseResponse{
			{Year: 2026, CBSTribIA: "90.00", CBSRFB: "90.00", IBSTribIA: "10.00", IBSRFB: "10.00", Divergente: false},
		},
		CasesTotal:     1,
		CasesDivergent: 0,
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var out EngineValidationResponse
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if !out.Validated {
		t.Error("validated deveria ser true no round-trip")
	}
	if out.Reference.Name != "Calculadora de Tributos RFB/Serpro" {
		t.Errorf("reference.name = %q", out.Reference.Name)
	}
	// A versão é o que separa "validado contra a versão X" de "validado" —
	// não pode se perder no round-trip (ver EngineValidationReferenceResponse).
	if out.Reference.Version != "1.0.0-beta" {
		t.Errorf("reference.version = %q, want %q", out.Reference.Version, "1.0.0-beta")
	}
	if len(out.Cases) != 1 || out.Cases[0].Year != 2026 {
		t.Errorf("round-trip de cases falhou: %+v", out.Cases)
	}
}

// TestEngineValidationHandler_SemEvidenciaDevolveValidatedFalse cobre o
// handler ponta a ponta: sem evidência gravada (o placeholder embutido por
// padrão), a rota nunca inventa validated:true nem reference preenchida.
func TestEngineValidationHandler_SemEvidenciaDevolveValidatedFalse(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/engine/validation", nil)
	w := httptest.NewRecorder()

	s.engineValidationHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var out EngineValidationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, w.Body.String())
	}
	if out.Validated {
		t.Error("validated deveria ser false sem evidência gravada")
	}
	if out.Reference.Name != "" || out.Reference.URL != "" || out.Reference.Version != "" {
		t.Errorf("reference deveria estar vazia sem validação: %+v", out.Reference)
	}
	if out.Cases == nil || out.Scope == nil || out.OutOfScope == nil {
		t.Error("slices não deveriam ser nil mesmo sem evidência")
	}
}

// TestEngineValidationHandler_MetodoNaoPermitido espelha a convenção de
// lawCorpusHandler: só GET.
func TestEngineValidationHandler_MetodoNaoPermitido(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/engine/validation", nil)
	w := httptest.NewRecorder()

	s.engineValidationHandler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}
