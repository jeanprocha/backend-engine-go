//go:build rfb

// Suíte cruzada contra a Calculadora de Tributos oficial da RFB/Serpro
// (W7/B2.1 — docs/roadmap-execucao.md). Atrás de build tag: não compila nem
// roda em `go test ./...` normal, não precisa de segredo nem serviço no CI
// (ver o comentário em .github/workflows/ci.yml, que já prescreve isso).
//
// Como rodar (validado ao vivo em 28/08/2026, W7/B2.1 — os 4 blocos abaixo
// que antes diziam "PREENCHER" já têm valores conferidos contra a API real,
// não placeholders):
//  1. Não precisa instalar nada: o default aponta para a API pública
//     hospedada no ambiente de homologação do piloto (rfbBaseURL()). Para
//     rodar contra uma instância offline/local, sobrescrever com
//     RFB_CALCULADORA_URL.
//  2. go test -tags=rfb ./internal/tax/... -run TestRFB -v
//  3. Para regravar a evidência (internal/enginevalidation/evidencia/
//     validacao_rfb.json), acrescentar -rfb-update. A versão da calculadora
//     é lida automaticamente do endpoint de dados abertos — não precisa de
//     RFB_CALCULADORA_VERSAO manual a menos que o path mude de novo.
//
// A API da RFB classifica por NCM (mercadoria) OU NBS — Nomenclatura
// Brasileira de Serviços (código de classificação tributária via
// CST/cClassTrib). TribIA modela só serviços: usa NBS, nunca NCM. O NBS,
// CST e cClassTrib do "serviço padrão, sem redução" abaixo foram obtidos
// direto da API oficial (GET dados-abertos/nbs/lista e
// dados-abertos/classificacoes-tributarias/nbs-aplicavel), não adivinhados
// nem copiados de exemplo de documentação — o achado anterior a esta PR foi
// exatamente esse: o placeholder era o NCM de CIGARRO do exemplo de
// mercadoria da doc (24021000), que a API aceitava sem erro (o cliente HTTP
// falava com a API corretamente) mas não representava serviço nenhum.
package tax_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jeanprocha/backend-engine-go/internal/enginevalidation"
	"github.com/jeanprocha/backend-engine-go/internal/tax"
	"github.com/shopspring/decimal"
)

// ─── Endpoint — validado ao vivo em 28/08/2026 (W7/B2.1) ──────────────────

// rfbBaseURL: override via RFB_CALCULADORA_URL, pensado para apontar a uma
// instância offline/local caso a API pública saia do ar ou mude de ambiente.
// Default é a API pública hospedada no ambiente de homologação do piloto
// ("apr" — confirmado via GET .../calculadora/dados-abertos/versao), não
// exige instalação nem autenticação.
func rfbBaseURL() string {
	if v := os.Getenv("RFB_CALCULADORA_URL"); v != "" {
		return v
	}
	return "https://piloto-cbs.tributos.gov.br/servico/calculadora-consumo/api"
}

// rfbRegimeGeralPath: caminho do endpoint de cálculo, relativo a rfbBaseURL().
const rfbRegimeGeralPath = "/calculadora/regime-geral"

// rfbCalculadoraVersaoPathDefault: caminho do recurso que informa a versão da
// calculadora e da base de alíquotas em vigor, relativo a rfbBaseURL().
const rfbCalculadoraVersaoPathDefault = "/calculadora/dados-abertos/versao"

// rfbCalculadoraVersaoPath: override via RFB_CALCULADORA_VERSAO_PATH, caso o
// path acima mude de novo no futuro sem precisar editar o arquivo.
func rfbCalculadoraVersaoPath() string {
	if v := os.Getenv("RFB_CALCULADORA_VERSAO_PATH"); v != "" {
		return v
	}
	return rfbCalculadoraVersaoPathDefault
}

// rfbVersaoResponse: shape real confirmado (não documentado no Swagger —
// obtido por chamada direta). Ex.: {"versaoApp":"1.3.0-af611293",
// "versaoDb":"V0042","descricaoVersaoDb":"...","dataVersaoDb":"2026-07-07",
// "ambiente":"apr"}. VersaoApp é a versão do serviço; VersaoDb identifica a
// base de alíquotas vigente (o que de fato importa para "contra qual tabela
// validamos") — rfbCalculadoraVersao() combina as duas num único carimbo.
type rfbVersaoResponse struct {
	VersaoApp    string `json:"versaoApp"`
	VersaoDb     string `json:"versaoDb"`
	DataVersaoDb string `json:"dataVersaoDb"`
	Ambiente     string `json:"ambiente"`
}

// ─── Classificação do "serviço padrão, sem redução" — validada ao vivo ────
// NBS, CST e cClassTrib abaixo vieram direto da API oficial (GET
// dados-abertos/nbs/lista e dados-abertos/classificacoes-tributarias/
// nbs-aplicavel), confirmados contra uma resposta 200 real do endpoint de
// cálculo — não são adivinhados nem copiados de exemplo de documentação.

const (
	// rfbServicoNBS: NBS 1.14.01.1-00 (serviços de tecnologia da informação,
	// sem redução), formato sem pontuação exigido pela API. CST 000 +
	// cClassTrib 000001 = "tributação integral" / "situações tributadas
	// integralmente pelo IBS e CBS" — o par confirmado como válido para este
	// NBS via o endpoint de validação nbs-aplicavel.
	rfbServicoNBS       = "114011100"
	rfbCSTPadrao        = "000"
	rfbCClassTribPadrao = "000001"
	rfbUnidadePadrao    = "UN"

	// rfbMunicipioIBGE / rfbUF: TribIA não coleta município no input — o IBS
	// tem componente municipal, então o resultado PODE variar por município.
	// Porto Alegre/RS, confirmado ao vivo: a alíquota municipal de referência
	// é idêntica à de São Paulo/SP em todos os anos da transição (nenhuma
	// variação por município ainda em vigor), então a escolha do município
	// "default" não afeta o resultado desta suíte hoje — mas pode passar a
	// afetar quando alíquotas municipais divergirem entre si.
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

// rfbItem: NCM (mercadoria) e NBS (serviço) são campos distintos e mutuamente
// exclusivos no schema real — TribIA só modela serviços, então preenche NBS
// e omite NCM (omitempty nos dois; nunca reusar um campo pelo outro).
type rfbItem struct {
	Numero            int                  `json:"numero"`
	NCM               string               `json:"ncm,omitempty"`
	NBS               string               `json:"nbs,omitempty"`
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
		Versao:          "1.0.0",
		DataHoraEmissao: dataHoraEmissao,
		Municipio:       rfbMunicipioIBGE,
		UF:              rfbUF,
		Itens: []rfbItem{
			{
				Numero:      1,
				NBS:         rfbServicoNBS,
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
		t.Fatalf("chamando %s: %v — API pública fora do ar, ou RFB_CALCULADORA_URL aponta para uma instância indisponível? Ver instruções no topo do arquivo.", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s devolveu %d (esperado 200) — inspecionar o Swagger em %s/api-docs para ver se o schema mudou", url, resp.StatusCode, rfbBaseURL())
	}

	var out rfbResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode resposta: %v", err)
	}
	return out
}

// Tipos de evidência vêm de internal/enginevalidation — mesmo pacote que
// GET /engine/validation embute e lê (go:embed exige um arquivo real no
// caminho de build; testdata/ não é incluído no binário de produção, por
// isso a evidência vive em internal/enginevalidation/evidencia/, não aqui).

var rfbUpdate = flag.Bool("rfb-update", false, "grava internal/enginevalidation/evidencia/validacao_rfb.json com a evidência da execução atual")

// TestRFB_RegimeGeral_ServicoPadrao compara, para cada ano 2026-2033, o CBS e
// o IBS que o motor TribIA calcula (via TaxComponents, W7/B2.1) contra o que
// a Calculadora oficial da RFB devolve para a mesma base de cálculo. Com
// -rfb-update, grava o resultado em internal/enginevalidation/evidencia/
// validacao_rfb.json — o artefato que GET /engine/validation lê para
// sustentar o selo (B2.3).
func TestRFB_RegimeGeral_ServicoPadrao(t *testing.T) {
	cases := loadCanonicalCases(t)
	c, ok := cases["empresa_servicos_padrao"]
	if !ok {
		t.Fatal("caso canônico \"empresa_servicos_padrao\" ausente de testdata/casos_canonicos.json")
	}

	calc := tax.NewCalculator()
	tolerance := decimal.RequireFromString("0.01")

	var evidence []enginevalidation.EvidenceCase
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

		evidence = append(evidence, enginevalidation.EvidenceCase{
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

// rfbCalculadoraVersao devolve a versão da Calculadora RFB usada nesta
// execução e, quando não consegue, o motivo (para a mensagem de erro de quem
// chama). NUNCA inventa nem devolve um default estático: uma versão fabricada
// é pior que versão nenhuma, porque o selo do dossiê a exibe como fato.
//
// O carimbo combina versaoApp (versão do serviço) e versaoDb/dataVersaoDb (a
// base de alíquotas vigente) — é essa segunda parte que de fato importa para
// "contra qual tabela validamos", já que a API pode trocar de base sem trocar
// de versaoApp.
//
// Ordem de resolução:
//  1. RFB_CALCULADORA_VERSAO — override manual, para quando o path abaixo
//     mudar de novo antes deste arquivo ser atualizado.
//  2. GET em rfbCalculadoraVersaoPath().
func rfbCalculadoraVersao() (versao string, motivo string) {
	if v := strings.TrimSpace(os.Getenv("RFB_CALCULADORA_VERSAO")); v != "" {
		return v, ""
	}

	url := rfbBaseURL() + rfbCalculadoraVersaoPath()
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Sprintf("GET %s falhou: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Sprintf("GET %s devolveu %d (o path pode ter mudado — sobrescrever com RFB_CALCULADORA_VERSAO_PATH)", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		return "", fmt.Sprintf("lendo resposta de %s: %v", url, err)
	}

	var payload rfbVersaoResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Sprintf("resposta de %s não é o JSON esperado (%v); corpo: %s", url, err, rfbTrecho(body))
	}
	app := strings.TrimSpace(payload.VersaoApp)
	db := strings.TrimSpace(payload.VersaoDb)
	data := strings.TrimSpace(payload.DataVersaoDb)
	if app == "" && db == "" {
		return "", fmt.Sprintf("resposta de %s não traz versaoApp nem versaoDb; corpo: %s", url, rfbTrecho(body))
	}
	switch {
	case app != "" && db != "" && data != "":
		return fmt.Sprintf("%s (base %s, %s)", app, db, data), ""
	case app != "" && db != "":
		return fmt.Sprintf("%s (base %s)", app, db), ""
	case app != "":
		return app, ""
	default:
		return db, ""
	}
}

// rfbTrecho recorta o corpo bruto para caber numa mensagem de erro, sem
// partir rune no meio.
func rfbTrecho(b []byte) string {
	const max = 200
	r := []rune(strings.TrimSpace(string(b)))
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return string(r)
}

func writeRFBEvidence(t *testing.T, cases []enginevalidation.EvidenceCase) {
	t.Helper()

	// A versão é obrigatória para gravar. A Calculadora RFB é beta e muda de
	// versão: um artefato que não diz contra QUAL versão rodou sustenta no
	// máximo "motor validado", e o selo do dossiê afirma "validado contra a
	// versão X". enginevalidation.Build já recusa Validated sem ela — falhar
	// aqui, alto, evita gravar em silêncio um artefato que nunca vai valer.
	versao, motivo := rfbCalculadoraVersao()
	if versao == "" {
		t.Fatalf("não foi possível determinar a versão da Calculadora RFB: %s\n"+
			"NADA foi gravado: sem a versão, o artefato não sustenta o selo (enginevalidation.Build exige calculadora_versao).\n"+
			"Saídas: (a) rodar de novo com RFB_CALCULADORA_VERSAO=<versão> como override manual; "+
			"(b) o path de %s pode ter mudado — verificar o Swagger em %s/api-docs e ajustar rfbCalculadoraVersaoPathDefault ou RFB_CALCULADORA_VERSAO_PATH.",
			motivo, rfbCalculadoraVersaoPathDefault, rfbBaseURL())
	}

	divergent := 0
	for _, c := range cases {
		if c.Divergente {
			divergent++
		}
	}
	manifest := enginevalidation.Manifest{
		ExecutadoEm:       time.Now().UTC().Format(time.RFC3339),
		CalculadoraURL:    rfbBaseURL(),
		CalculadoraVersao: versao,
		Escopo:            []string{"CBS", "IBS", "regime regular (empresa de serviços)"},
		ForaDoEscopo: []string{
			"PIS/COFINS", "ISS", "ICMS", "IPI", "Imposto Seletivo",
			"Simples Nacional", "MEI", "prof_liberal (premissa TribIA, sem base legal)",
			"alíquotas municipais fora de " + rfbUF,
		},
		Tolerancia:          "0.01",
		Casos:               cases,
		CasosTotal:          len(cases),
		CasosDivergem:       divergent,
		TabelaTransicaoHash: tax.TransitionTableHash(),
	}
	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	// internal/enginevalidation/evidencia/, não testdata/ — go:embed no
	// pacote de produção não inclui testdata (ver comentário no topo do arquivo).
	path := filepath.Join("..", "enginevalidation", "evidencia", "validacao_rfb.json")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("escrevendo %s: %v", path, err)
	}
	t.Logf("evidência gravada em %s (%d casos, %d divergentes)", path, len(cases), divergent)
}
