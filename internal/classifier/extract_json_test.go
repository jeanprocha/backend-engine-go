package classifier

import (
	"encoding/json"
	"testing"
)

// extractJSONObject é a rede que impede a resposta da LLM de derrubar a
// classificação inteira. Cada caso abaixo saiu de produção ou é o formato que
// o parser precisa aguentar.
func TestExtractJSONObject(t *testing.T) {
	casos := []struct {
		nome string
		in   string
		want string
	}{
		{
			// O caso que motivou a correção: ~40% das respostas em produção
			// vinham com uma chave de fechamento a mais (Onda 2/PR 6). A versão
			// anterior (primeiro "{" até o ÚLTIMO "}") devolvia a string
			// inalterada — o reparo era no-op e a classificação falhava.
			nome: "chave de fechamento a mais no fim",
			in:   `{"is_eligible":true,"confidence":0.85}}`,
			want: `{"is_eligible":true,"confidence":0.85}`,
		},
		{
			nome: "duas chaves a mais",
			in:   `{"a":1}}}`,
			want: `{"a":1}`,
		},
		{
			nome: "objeto puro, sem lixo",
			in:   `{"a":1,"b":{"c":2}}`,
			want: `{"a":1,"b":{"c":2}}`,
		},
		{
			nome: "prosa antes e depois",
			in:   "Claro! Segue:\n{\"a\":1}\nEspero ter ajudado.",
			want: `{"a":1}`,
		},
		{
			// Objeto aninhado no fim não pode ser truncado: a chave que fecha o
			// externo vem depois da que fecha o interno.
			nome: "objeto aninhado no fim + chave extra",
			in:   `{"a":1,"span":{"start":0,"end":38}}}`,
			want: `{"a":1,"span":{"start":0,"end":38}}`,
		},
		{
			nome: "chave dentro de string não confunde",
			in:   `{"just":"tem } aqui dentro"}}`,
			want: `{"just":"tem } aqui dentro"}`,
		},
		{
			nome: "sem objeto nenhum",
			in:   "não consegui responder",
			want: "",
		},
		{
			nome: "objeto incompleto (truncado) não é reparável",
			in:   `{"a":1,`,
			want: "",
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got := extractJSONObject(c.in)
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
			if got == "" {
				return
			}
			// O que sai tem que ser JSON válido — é o contrato com o chamador.
			var v map[string]any
			if err := json.Unmarshal([]byte(got), &v); err != nil {
				t.Errorf("saída não é JSON válido: %v", err)
			}
		})
	}
}

// A resposta real que estava falhando em produção, com o shape completo que o
// classificador espera — prova que o reparo devolve algo que desserializa no
// tipo de destino, não só num map genérico.
func TestExtractJSONObject_RespostaRealDeProducao(t *testing.T) {
	bruto := `{"is_eligible":true,"confidence":0.85,"justification":"Licença de software ERP é essencial para a atividade.","risk_level":"medio","regime_type":"padrao","evidence":[{"article_id":"lc214_0048_art_47_p1","snippets":["uso ou consumo pessoal"],"snippets_tentative":["apropriação dos créditos"]}],"matched_span":{"start":0,"end":38},"suggested_tags":[]}}`

	reparado := extractJSONObject(bruto)
	if reparado == "" {
		t.Fatal("não conseguiu isolar o objeto")
	}
	var resp classificationLLMResponse
	if err := json.Unmarshal([]byte(reparado), &resp); err != nil {
		t.Fatalf("não desserializa no tipo do classificador: %v", err)
	}
	if !resp.IsEligible {
		t.Error("is_eligible perdido no reparo")
	}
}
