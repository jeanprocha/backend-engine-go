package classifier

import "testing"

// TestStripCodeFence cobre os invólucros de cerca de código que a LLM ainda
// pode devolver mesmo com response_format json_object ligado em ClassifyChat.
func TestStripCodeFence(t *testing.T) {
	casos := []struct {
		nome string
		in   string
		want string
	}{
		{"sem cerca", `{"a":1}`, `{"a":1}`},
		{"cerca com json minúsculo", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"cerca com JSON maiúsculo", "```JSON\n{\"a\":1}\n```", `{"a":1}`},
		{"cerca com espaço antes do idioma", "``` json\n{\"a\":1}\n```", `{"a":1}`},
		{"cerca sem idioma", "```\n{\"a\":1}\n```", `{"a":1}`},
		{"cerca sem quebra de linha final antes do fechamento", "```json\n{\"a\":1}```", `{"a":1}`},
		{"espaços ao redor", "  \n```json\n{\"a\":1}\n```  \n", `{"a":1}`},
		{
			// Sem cerca, mas com "{" bem no início: IndexByte encontraria um
			// '\n' só depois do primeiro objeto — a janela curta (<=12) evita
			// tratar conteúdo real como marca de idioma.
			nome: "chave logo após crase tripla sem idioma nem quebra de linha próxima",
			in:   "```{\"justification\":\"texto bem mais longo que doze caracteres antes da primeira quebra\"}\n```",
			want: `{"justification":"texto bem mais longo que doze caracteres antes da primeira quebra"}`,
		},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := stripCodeFence(c.in); got != c.want {
				t.Fatalf("stripCodeFence(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestParseClassificationResponse cobre os três desfechos possíveis: parse
// direto, parse via reparo (extractJSONObject) e falha honesta quando nada
// funciona — o contrato que ClassifyExpense usa para decidir a re-tentativa.
func TestParseClassificationResponse(t *testing.T) {
	t.Run("parse direto", func(t *testing.T) {
		resp, cleaned, err := parseClassificationResponse(`{"is_eligible":true,"confidence":0.9}`)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if !resp.IsEligible || resp.Confidence != 0.9 {
			t.Errorf("resp = %+v", resp)
		}
		if cleaned != `{"is_eligible":true,"confidence":0.9}` {
			t.Errorf("cleaned = %q", cleaned)
		}
	})

	t.Run("cerca de markdown + parse direto", func(t *testing.T) {
		resp, _, err := parseClassificationResponse("```json\n{\"is_eligible\":false}\n```")
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if resp.IsEligible {
			t.Errorf("resp = %+v", resp)
		}
	})

	t.Run("chave de fechamento a mais precisa do reparo", func(t *testing.T) {
		resp, _, err := parseClassificationResponse(`{"is_eligible":true,"confidence":0.7}}`)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if !resp.IsEligible {
			t.Errorf("resp = %+v", resp)
		}
	})

	t.Run("cerca + chave de fechamento a mais (os dois problemas juntos)", func(t *testing.T) {
		resp, _, err := parseClassificationResponse("```json\n{\"is_eligible\":true,\"confidence\":0.5}}\n```")
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if !resp.IsEligible {
			t.Errorf("resp = %+v", resp)
		}
	})

	t.Run("json truncado não é reparável — erro honesto", func(t *testing.T) {
		_, _, err := parseClassificationResponse(`{"is_eligible":true,"confidence":`)
		if err == nil {
			t.Fatal("esperava erro para JSON truncado (finish_reason=length não tem reparo possível)")
		}
	})

	t.Run("sem json nenhum — erro honesto", func(t *testing.T) {
		_, _, err := parseClassificationResponse("não consegui responder a esta pergunta")
		if err == nil {
			t.Fatal("esperava erro")
		}
	})
}
