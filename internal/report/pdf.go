// Package report gera PDFs de diagnóstico (Maroto v2) a partir do histórico persistido.
package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/jeanprocha/backend-engine-go/internal/history"
	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
	"github.com/shopspring/decimal"
)

// GenerateDiagnosticPDF monta o parecer técnico ilustrativo a partir de uma simulação gravada.
func GenerateDiagnosticPDF(d *history.Detail) ([]byte, error) {
	if d == nil {
		return nil, fmt.Errorf("report: detail vazio")
	}

	cfg := config.NewBuilder().
		WithTitle("Diagnostico TribIA", true).
		WithSubject("Simulacao reforma tributaria LC 68/2024", true).
		Build()

	m := maroto.New(cfg)

	addSectionTitle(m, "Diagnostico TribIA — Reforma tributaria")
	m.AddRow(5, text.NewCol(12, "Documento ilustrativo. Nao substitui assessoria fiscal ou contabil.", props.Text{Size: 8, Style: fontstyle.Italic}))

	meta := fmt.Sprintf("Gerado em: %s | Ano da simulacao: %d | ID: %s",
		time.Now().Format("02/01/2006 15:04"), d.Year, d.ID.String())
	m.AddRow(6, text.NewCol(12, meta, props.Text{Size: 8}))

	addDocumentLens(m, d)

	buildExecutiveSummary(m, d)
	addCreditorSaldoNoteIfApplicable(m, d)
	buildServicesTable(m, d)
	buildCreditsDetail(m, d)

	doc, err := m.Generate()
	if err != nil {
		return nil, fmt.Errorf("report: gerar pdf: %w", err)
	}
	return doc.GetBytes(), nil
}

func addSectionTitle(m core.Maroto, title string) {
	m.AddRow(10, text.NewCol(12, title, props.Text{Size: 14, Style: fontstyle.Bold}))
}

// addDocumentLens expõe perfil do simulador e contexto textual (lente de análise).
func addDocumentLens(m core.Maroto, d *history.Detail) {
	reg := strings.TrimSpace(d.Simulation.CompanyRegime)
	ctx := strings.TrimSpace(d.CompanyContext)
	if reg != "" {
		line := fmt.Sprintf("Perfil simulador: %s", reg)
		m.AddAutoRow(text.NewCol(12, line, props.Text{Size: 9, Style: fontstyle.Italic}))
	}
	if ctx != "" {
		m.AddAutoRow(text.NewCol(12, truncateRunes(ctx, 400), props.Text{Size: 8, Style: fontstyle.Italic}))
	}
}

// addCreditorSaldoNoteIfApplicable destaca perfis com saida desonerada e credito nas entradas (ilustrativo).
func addCreditorSaldoNoteIfApplicable(m core.Maroto, d *history.Detail) {
	reg := strings.ToLower(strings.TrimSpace(d.Simulation.CompanyRegime))
	if reg != "exportadora" && reg != "aliquota_zero" {
		return
	}
	rule := "Regra de calculo (exportacao / saida com aliquota efetiva 0% neste perfil): " +
		"de forma conceitual, Liquido = (Receita x 0% na tributacao de saida) - Creditos. " +
		"No quadro acima, Bruto, Creditos e Liquido seguem o modelo ilustrativo Liquido = Bruto - Creditos: " +
		"com saida zerada ou desonerada, Bruto projetado tende a refletir ausencia de carga na saida e " +
		"Liquido incorpora a manutencao de credito nas entradas."
	m.AddAutoRow(text.NewCol(12, rule, props.Text{Size: 8, Style: fontstyle.Italic}))
	saldo := "Liquido negativo nao e erro de calculo: indica Saldo credor ilustrativo " +
		"(credito acumulado nas entradas elegiveis), posicao financeira favoravel no cenario do simulador, " +
		"sem substituir compensacao ou ressarcimento na pratica operacional."
	m.AddAutoRow(text.NewCol(12, saldo, props.Text{Size: 8, Style: fontstyle.Italic}))
}

func buildExecutiveSummary(m core.Maroto, d *history.Detail) {
	addSectionTitle(m, "Resumo tributario (valores em BRL)")
	sim := d.Simulation

	m.AddRow(7,
		col.New(4).Add(text.New("", props.Text{Size: 9, Style: fontstyle.Bold})),
		col.New(4).Add(text.New("Atual (ilustrativo)", props.Text{Size: 9, Style: fontstyle.Bold, Align: align.Center})),
		col.New(4).Add(text.New("Projetado CBS/IBS", props.Text{Size: 9, Style: fontstyle.Bold, Align: align.Center})),
	)
	m.AddRow(6,
		col.New(4).Add(text.New("Bruto", props.Text{Size: 9})),
		col.New(4).Add(text.New(formatBRL(sim.Current.GrossTax), props.Text{Size: 9, Align: align.Right})),
		col.New(4).Add(text.New(formatBRL(sim.Projected.GrossTax), props.Text{Size: 9, Align: align.Right})),
	)
	m.AddRow(6,
		col.New(4).Add(text.New("Creditos", props.Text{Size: 9})),
		col.New(4).Add(text.New(formatBRL(sim.Current.Credits), props.Text{Size: 9, Align: align.Right})),
		col.New(4).Add(text.New(formatBRL(sim.Projected.Credits), props.Text{Size: 9, Align: align.Right})),
	)
	m.AddRow(6,
		col.New(4).Add(text.New("Liquido", props.Text{Size: 9, Style: fontstyle.Bold})),
		col.New(4).Add(text.New(formatBRL(sim.Current.NetTax), props.Text{Size: 9, Style: fontstyle.Bold, Align: align.Right})),
		col.New(4).Add(text.New(formatBRL(sim.Projected.NetTax), props.Text{Size: 9, Style: fontstyle.Bold, Align: align.Right})),
	)
	m.AddRow(6,
		col.New(4).Add(text.New("Delta / Var. %", props.Text{Size: 9})),
		col.New(8).Add(text.New(formatBRL(sim.Delta)+" ("+strings.TrimSpace(sim.DeltaPct)+"%)", props.Text{Size: 9, Align: align.Left})),
	)
	m.AddRow(5, text.NewCol(12,
		"Equacao ilustrativa do simulador: Liquido = Bruto - Creditos (conferir colunas Bruto, Creditos e Liquido).",
		props.Text{Size: 7, Style: fontstyle.Italic},
	))
}

func buildServicesTable(m core.Maroto, d *history.Detail) {
	if len(d.Services) == 0 {
		return
	}
	addSectionTitle(m, "Receitas (servicos informados)")
	m.AddRow(7,
		col.New(6).Add(text.New("Descricao", props.Text{Size: 9, Style: fontstyle.Bold})),
		col.New(3).Add(text.New("Valor", props.Text{Size: 9, Style: fontstyle.Bold, Align: align.Right})),
		col.New(3).Add(text.New("Alq. ISS", props.Text{Size: 9, Style: fontstyle.Bold, Align: align.Right})),
	)
	for _, s := range d.Services {
		m.AddRow(6,
			col.New(6).Add(text.New(truncateRunes(s.Description, 80), props.Text{Size: 8})),
			col.New(3).Add(text.New(formatBRL(s.Amount), props.Text{Size: 8, Align: align.Right})),
			col.New(3).Add(text.New(formatRatePercent(s.ISSRate), props.Text{Size: 8, Align: align.Right})),
		)
	}
}

func buildCreditsDetail(m core.Maroto, d *history.Detail) {
	if len(d.Expenses) == 0 {
		addSectionTitle(m, "Despesas e creditos (IA)")
		m.AddRow(6, text.NewCol(12, "Nenhuma despesa registrada nesta simulacao.", props.Text{Size: 9}))
		return
	}

	classByDesc := make(map[string]history.ClassificationLine, len(d.Classifications))
	for _, c := range d.Classifications {
		classByDesc[c.Description] = c
	}

	addSectionTitle(m, "Analise de despesas e creditos (classificacao assistida)")
	m.AddRow(6, text.NewCol(12, "Elegibilidade e fundamentacao conforme dados persistidos; trechos RAG completos nao estao no historico.", props.Text{Size: 7, Style: fontstyle.Italic}))

	m.AddRow(7,
		col.New(3).Add(text.New("Descricao", props.Text{Size: 8, Style: fontstyle.Bold})),
		col.New(2).Add(text.New("Valor", props.Text{Size: 8, Style: fontstyle.Bold, Align: align.Right})),
		col.New(1).Add(text.New("Status", props.Text{Size: 7, Style: fontstyle.Bold, Align: align.Center})),
		col.New(2).Add(text.New("Regime", props.Text{Size: 7, Style: fontstyle.Bold})),
		col.New(2).Add(text.New("Base legal", props.Text{Size: 8, Style: fontstyle.Bold})),
		col.New(2).Add(text.New("Justificativa", props.Text{Size: 8, Style: fontstyle.Bold})),
	)

	cellPad := props.Text{Size: 7, Top: 0.5, VerticalPadding: 0.5}
	cellLegal := props.Text{Size: 7, Top: 0.5, VerticalPadding: 0.5, Style: fontstyle.Bold}
	inadmissivelColor := props.Color{Red: 55, Green: 55, Blue: 55}

	for _, exp := range d.Expenses {
		cl := classByDesc[exp.Description]
		var statusLabel string
		var statusProps props.Text
		if exp.IsEligible {
			statusLabel = "Elegivel"
			statusProps = props.Text{Size: 7, Align: align.Center, Top: 0.5, VerticalPadding: 0.5, Color: &props.GreenColor}
		} else {
			statusLabel = "Inadmissivel"
			c := inadmissivelColor
			statusProps = props.Text{Size: 7, Align: align.Center, Top: 0.5, VerticalPadding: 0.5, Color: &c}
		}
		legal := strings.TrimSpace(cl.LegalBase)
		if legal == "" {
			legal = "—"
		}
		just := strings.TrimSpace(cl.Justification)
		if just == "" {
			just = "—"
		}
		just = stripRedundantLegalEchoForReport(legal, just)
		reg := strings.TrimSpace(cl.RegimeType)
		if reg == "" {
			reg = "padrao"
		}
		m.AddAutoRow(
			col.New(3).Add(text.New(truncateRunes(exp.Description, 60), cellPad)),
			col.New(2).Add(text.New(formatBRL(exp.Amount), props.Text{Size: 7, Align: align.Right, Top: 0.5, VerticalPadding: 0.5})),
			col.New(1).Add(text.New(statusLabel, statusProps)),
			col.New(2).Add(text.New(reg, cellPad)),
			col.New(2).Add(text.New(legal, cellLegal)),
			col.New(2).Add(text.New(just, cellPad)),
		)
	}
}

// stripRedundantLegalEchoForReport reduz eco entre base legal e justificativa em dados já persistidos.
func stripRedundantLegalEchoForReport(legalBase, justification string) string {
	legal := strings.TrimSpace(legalBase)
	just := strings.TrimSpace(justification)
	if legal == "" || legal == "—" || just == "" || just == "—" {
		return just
	}
	lowL, lowJ := strings.ToLower(legal), strings.ToLower(just)
	if strings.HasPrefix(lowJ, lowL) {
		rest := strings.TrimSpace(just[len(legal):])
		rest = strings.TrimLeft(rest, " ,.;—–-")
		if rest != "" {
			return rest
		}
	}
	tail := ", conforme " + legal
	if len(just) > len(tail) && strings.HasSuffix(lowJ, strings.ToLower(tail)) {
		return strings.TrimSpace(just[:len(just)-len(tail)])
	}
	tail2 := " conforme " + legal
	if len(just) > len(tail2) && strings.HasSuffix(lowJ, strings.ToLower(tail2)) {
		return strings.TrimSpace(just[:len(just)-len(tail2)])
	}
	return just
}

func formatBRL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "R$ 0,00"
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return s
	}
	neg := d.IsNegative()
	abs := d.Abs().Round(2)
	str := abs.StringFixed(2)
	intPart, frac := str, "00"
	if i := strings.IndexByte(str, '.'); i >= 0 {
		intPart, frac = str[:i], str[i+1:]
	}
	if intPart == "" || intPart == "-" {
		intPart = "0"
	}
	var b strings.Builder
	n := len(intPart)
	for j, r := range intPart {
		if j > 0 && (n-j)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	out := "R$ " + b.String() + "," + frac
	if neg {
		return "-" + out
	}
	return out
}

func formatRatePercent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "—"
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return s
	}
	pct := d.Mul(decimal.NewFromInt(100)).StringFixed(1)
	return pct + "%"
}

func truncateRunes(s string, max int) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max-3]) + "..."
}
