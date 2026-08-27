//go:build rfb

// Suíte cruzada contra a Calculadora de Tributos oficial da RFB/Serpro
// (W7/B2.1 — docs/roadmap-execucao.md). Atrás de build tag: não compila nem
// roda em `go test ./...` normal, não precisa de segredo nem serviço no CI
// (ver o comentário em .github/workflows/ci.yml, que já prescreve isso).
//
// Como rodar:
//  1. Instalar e subir a Calculadora offline (Docker ou JAR — ver
//     https://piloto-cbs.tributos.gov.br/servico/calculadora-consumo/calculadora/calculadora-offline).
//  2. Confirmar a URL e o path exatos abrindo http://localhost:8080/api no
//     navegador (Swagger) — a RFB_ITEM_ENDPOINT_PATH abaixo é um palpite
//     razoável, não confirmado contra uma instância real.
//  3. go test -tags=rfb ./internal/tax/... -run TestRFB -v
//
// ⚠️ ANTES DE RODAR PARA VALER: os quatro blocos "PREENCHER" abaixo têm
// placeholders, não valores verificados. A API da RFB classifica por NCM
// (mercadoria) + CST/cClassTrib (código de classificação tributária); TribIA
// modela só serviços, que usam NBS (Nomenclatura Brasileira de Serviços) e
// não NCM. Não adivinhe o cClassTrib de "serviço padrão, sem redução" — a
// forma mais confiável de descobrir é abrir a calculadora WEB
// (http://localhost:80), simular manualmente uma operação de serviço padrão,
// e inspecionar a requisição real que o formulário envia (DevTools → Rede).
// Copie os valores exatos para as constantes abaixo antes de confiar em
// qualquer resultado desta suíte. Ver também o Informe Técnico RT 2025.002
// (tabela de correlação LC 116/2003 × NBS × cClassTrib, Anexo VIII).
package tax_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jeanprocha/backend-engine-go/internal/tax"
	"github.com/shopspring/decimal"
)

// ─── PREENCHER: endpoint ──────────────────────────────────────────────────

// rfbBaseURL: override via RFB_CALCULADORA_URL. Default é um palpite — a
// documentação consultada tem duas indicações conflitantes sobre o path
// local exato (ver comentário do pacote); confirmar contra o Swagger antes
// de confiar.
func rfbBaseURL() string {
	if v := os.Getenv("RFB_CALCULADORA_URL"); v != "" {
		return v
	}
	return "http://localhost:8080/api"
}

// rfbRegimeGeralPath: caminho do endpoint de cálculo, relativo a rfbBaseURL().
// TODO(W7/B2.1): confirmar contra o Swagger de uma instância local real.
const rfbRegimeGeralPath = "/calculadora/regime-geral"

// ─── PREENCHER: classificação do "serviço padrão, sem redução" ───────────
// TODO(W7/B2.1): valores abaixo são placeholders copiados do exemplo de
// MERCADORIA da documentação (cigarro, NCM 24021000) — servem só para provar
// que o cliente HTTP fala com a API corretamente. NÃO representam um serviço
// real. Substituir antes de tirar qualquer conclusão de validação.

const (
	// rfbServicoNBSPlaceholder: código NBS do serviço "padrão" simulado.
	// TODO: descobrir o NBS real (ex.: consultoria = NBS 1.0107 ss., a
	// confirmar) e o cClassTrib correlato via Anexo VIII / RT 2025.002.
	rfbServicoNBSPlaceholder = "24021000" // placeholder — é um NCM de mercadoria, não NBS de serviço
	rfbCSTPadrao             = "000"      // placeholder — CST do exemplo de mercadoria
	rfbCClassTribPadrao      = "000001"   // placeholder — cClassTrib do exemplo de mercadoria
	rfbUnidadePadrao         = "UN"       // placeholder — unidade "genérica"; confirmar se a API aceita

	// rfbMunicipioIBGE / rfbUF: TribIA não coleta município no input — o
	// IBS tem componente municipal, então o resultado pode variar por
	// município. Placeholder = Porto Alegre/RS (o mesmo do exemplo da
	// documentação). Escolher o município "default" da comparação é decisão
	// de produto, não só técnica — revisar antes de publicar o selo.
	rfbMunicipioIBGE = 4314902
	rfbUF            = "RS"
)

// ─── Tipos — schema confirmado contra a documentação oficial (payload de
// exemplo do endpoint /calculadora/regime-geral, requisição e resposta). ───

type rfbRequest struct {
	ID              string    `json:"id"`
	Versao          string    `json:"versao"`
	DataHoraEmissao string    `json:"dataHoraEmissao"`
	Municipio       int       `json:"municipio"`
	UF              string    `json:"uf"`
	Itens           []rfbItem `json:"itens"`
}

type rfbItem struct {
	Numero            int                  `json:"numero"`
	NCM               string               `json:"ncm"`
	CST               string               `json:"cst"`
	BaseCalculo       json.RawMessage      `json:"baseCalculo"`
	Quantidade        json.RawMessage      `json:"quantidade"`
	Unidade           string               `json:"unidade"`
	TributacaoRegular rfbTributacaoRegular `json:"tributacaoRegular"`
	CClassTrib        string               `json:"cClassTrib"`
}

type rfbTributacaoRegular struct {
	CST        string `json:"cst"`
	CClassTrib string `json:"cClassTrib"`
}

type rfbResponse struct {
	Objetos []rfbObjeto  `json:"objetos"`
	Total   rfbTotalWrap `json:"total"`
}

type rfbObjeto struct {
	NObj     int `json:"nObj"`
	TribCalc struct {
		IBSCBS struct {
			GIBSCBS struct {
				GCBS struct {
					PCBS string `json:"pCBS"`
					VCBS string `json:"vCBS"`
				} `json:"gCBS"`
				GIBSUF struct {
					PIBSUF string `json:"pIBSUF"`
					VIBSUF string `json:"vIBSUF"`
				} `json:"gIBSUF"`
				GIBSMun struct {
					PIBSMun string `json:"pIBSMun"`
					VIBSMun string `json:"vIBSMun"`
				} `json:"gIBSMun"`
			} `json:"gIBSCBS"`
		} `json:"IBSCBS"`
	} `json:"tribCalc"`
}

type rfbTotalWrap struct {
	TribCalc struct {
		IBSCBSTot struct {
			VBCIBSCBS string `json:"vBCIBSCBS"`
			GIBS      struct {
				VIBS string `json:"vIBS"`
			} `json:"gIBS"`
			GCBS struct {
				VCBS string `json:"vCBS"`
			} `json:"gCBS"`
		} `json:"IBSCBSTot"`
	} `json:"tribCalc"`
}

// decimalToRawJSON serializa um decimal.Decimal como número JSON literal
// (não string, não float64 — nunca passa por ponto flutuante binário; ver a
// proibição de float64 no domínio fiscal, CLAUDE.md/README).
func decimalToRawJSON(d decimal.Decimal) json.RawMessage {
	return json.RawMessage(d.String())
}

// callRegimeGeral envia UMA operação de serviço padrão (valor em amount, sem
// desconto) e devolve a resposta bruta.
func callRegimeGeral(t *testing.T, amount decimal.Decimal, dataHoraEmissao string) rfbResponse {
	t.Helper()
	req := rfbRequest{
		ID:              fmt.Sprintf("tribia-w7-%d", time.Now().UnixNano()),
		Versao:          "0.0.1",
		DataHoraEmissao: dataHoraEmissao,
		Municipio:       rfbMunicipioIBGE,
		UF:              rfbUF,
		Itens: []rfbItem{
			{
				Numero:      1,
				NCM:         rfbServicoNBSPlaceholder,
				CST:         rfbCSTPadrao,
				BaseCalculo: decimalToRawJSON(amount),
				Quantidade:  json.RawMessage("1"),
				Unidade:     rfbUnidadePadrao,
				TributacaoRegular: rfbTributacaoRegular{
					CST:        rfbCSTPadrao,
					CClassTrib: rfbCClassTribPadrao,
				},
				CClassTrib: rfbCClassTribPadrao,
			},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	url := rfbBaseURL() + rfbRegimeGeralPath
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("montar requisição: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("chamando %s: %v — a Calculadora está rodando localmente? Ver instruções no topo do arquivo.", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s devolveu %d (esperado 200) — payload de exemplo em %s pode não corresponder à API real; abra o Swagger em %s/api", url, resp.StatusCode, url, rfbBaseURL())
	}

	var out rfbResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode resposta: %v", err)
	}
	return out
}

// rfbEvidenceCase é um veredito por (ano); grava o artefato que sustenta o
// selo do B2.3 — GET /engine/validation lê isto, nunca uma string escrita à
// mão (mesma regra de ouro de internal/lawcorpus: fato de execução, não
// release note inventada).
type rfbEvidenceCase struct {
	Year        int    `json:"year"`
	CBSTribIA   string `json:"cbs_tribia"`
	CBSRFB      string `json:"cbs_rfb"`
	IBSTribIA   string `json:"ibs_tribia"`
	IBSRFB      string `json:"ibs_rfb"`
	Divergente  bool   `json:"divergente"`
	Observacoes string `json:"observacoes,omitempty"`
}

type rfbEvidenceManifest struct {
	ExecutadoEm    string            `json:"executado_em"`
	CalculadoraURL string            `json:"calculadora_url"`
	Escopo         []string          `json:"escopo"`
	ForaDoEscopo   []string          `json:"fora_do_escopo"`
	Tolerancia     string            `json:"tolerancia_brl"`
	Casos          []rfbEvidenceCase `json:"casos"`
	CasosTotal     int               `json:"casos_total"`
	CasosDivergem  int               `json:"casos_divergentes"`
}

var rfbUpdate = flag.Bool("rfb-update", false, "grava internal/tax/testdata/validacao_rfb.json com a evidência da execução atual")

// TestRFB_RegimeGeral_ServicoPadrao compara, para cada ano 2026-2033, o CBS e
// o IBS que o motor TribIA calcula (via TaxComponents, W7/B2.1) contra o que
// a Calculadora oficial da RFB devolve para a mesma base de cálculo. Com
// -rfb-update, grava o resultado em testdata/validacao_rfb.json — o artefato
// que GET /engine/validation lê para sustentar o selo (B2.3).
//
// ⚠️ Só produz um veredito confiável depois que os blocos "PREENCHER" no topo
// do arquivo tiverem valores verificados contra uma instância real — com os
// placeholders atuais, este teste prova só que o cliente HTTP fala com a API
// corretamente, não que o motor está certo.
func TestRFB_RegimeGeral_ServicoPadrao(t *testing.T) {
	cases := loadCanonicalCases(t)
	c, ok := cases["empresa_servicos_padrao"]
	if !ok {
		t.Fatal("caso canônico \"empresa_servicos_padrao\" ausente de testdata/casos_canonicos.json")
	}

	calc := tax.NewCalculator()
	tolerance := decimal.RequireFromString("0.01")

	var evidence []rfbEvidenceCase
	for year := 2026; year <= 2033; year++ {
		input := c.toInput(year)
		result, err := calc.Calculate(context.Background(), input)
		if err != nil {
			t.Fatalf("year=%d: Calculate: %v", year, err)
		}

		// Uma emissão em 1º de janeiro do ano em foco — a RFB pode aplicar a
		// tabela vigente na dataHoraEmissao; não em RFB_Versao ou em "hoje".
		dataHoraEmissao := fmt.Sprintf("%d-01-15T09:00:00-03:00", year)
		amount := input.Services[0].Amount // caso canônico tem 1 serviço só (ver testdata)

		resp := callRegimeGeral(t, amount, dataHoraEmissao)
		if len(resp.Objetos) == 0 {
			t.Fatalf("year=%d: resposta sem objetos", year)
		}
		g := resp.Objetos[0].TribCalc.IBSCBS.GIBSCBS

		cbsRFB, err := decimal.NewFromString(g.GCBS.VCBS)
		if err != nil {
			t.Fatalf("year=%d: vCBS inválido %q: %v", year, g.GCBS.VCBS, err)
		}
		// IBS = componente estadual + municipal (a API os separa; TribIA soma).
		ibsUF, err := decimal.NewFromString(g.GIBSUF.VIBSUF)
		if err != nil {
			t.Fatalf("year=%d: vIBSUF inválido %q: %v", year, g.GIBSUF.VIBSUF, err)
		}
		ibsMun, err := decimal.NewFromString(g.GIBSMun.VIBSMun)
		if err != nil {
			t.Fatalf("year=%d: vIBSMun inválido %q: %v", year, g.GIBSMun.VIBSMun, err)
		}
		ibsRFB := ibsUF.Add(ibsMun)

		cbsTribIA := result.Projected.Components.CBS
		ibsTribIA := result.Projected.Components.IBS

		divergente := cbsTribIA.Sub(cbsRFB).Abs().GreaterThan(tolerance) ||
			ibsTribIA.Sub(ibsRFB).Abs().GreaterThan(tolerance)

		evidence = append(evidence, rfbEvidenceCase{
			Year:       year,
			CBSTribIA:  cbsTribIA.StringFixed(2),
			CBSRFB:     cbsRFB.StringFixed(2),
			IBSTribIA:  ibsTribIA.StringFixed(2),
			IBSRFB:     ibsRFB.StringFixed(2),
			Divergente: divergente,
		})

		if divergente {
			t.Errorf("year=%d: CBS tribia=%s rfb=%s | IBS tribia=%s rfb=%s (tolerância %s)",
				year, cbsTribIA, cbsRFB, ibsTribIA, ibsRFB, tolerance)
		}
	}

	if *rfbUpdate {
		writeRFBEvidence(t, evidence)
	}
}

func writeRFBEvidence(t *testing.T, cases []rfbEvidenceCase) {
	t.Helper()
	divergent := 0
	for _, c := range cases {
		if c.Divergente {
			divergent++
		}
	}
	manifest := rfbEvidenceManifest{
		ExecutadoEm:    time.Now().UTC().Format(time.RFC3339),
		CalculadoraURL: rfbBaseURL(),
		Escopo:         []string{"CBS", "IBS", "regime regular (empresa de serviços)"},
		ForaDoEscopo: []string{
			"PIS/COFINS", "ISS", "ICMS", "IPI", "Imposto Seletivo",
			"Simples Nacional", "MEI", "prof_liberal (premissa TribIA, sem base legal)",
			"alíquotas municipais fora de " + rfbUF,
		},
		Tolerancia:    "0.01",
		Casos:         cases,
		CasosTotal:    len(cases),
		CasosDivergem: divergent,
	}
	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	path := filepath.Join("testdata", "validacao_rfb.json")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("escrevendo %s: %v", path, err)
	}
	t.Logf("evidência gravada em %s (%d casos, %d divergentes)", path, len(cases), divergent)
}
