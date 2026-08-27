// Package lawcorpus mescla o que ESTÁ ingerido em tax_law_chunks (via
// ingestion.CorpusDocumentRow) com o conhecimento estático sobre cada
// documento legal (rótulo bonito, URL da fonte oficial, data de publicação)
// para montar a resposta de GET /law/corpus.
//
// Regra dura: o changelog produzido aqui é sempre um FATO de indexação
// ("N trechos, data-base DD/MM/AAAA"), nunca uma release note inventada —
// PRODUCT.md exige que a UI não exiba selos que o backend não sustenta.
package lawcorpus

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
)

// DefaultSourceLabel é o rótulo do documento que ESTÁ no banco hoje —
// duplica ingestion.DefaultDocumentProfile().SourceLabel (não importa
// ingestion "de volta" para não impor essa dependência a quem só quer o
// catálogo; os dois valores precisam ficar sincronizados manualmente até a
// Onda 2 os unificar).
const DefaultSourceLabel = "LC 68/2024"

// DocMeta é o conhecimento estático sobre um documento legal — o que não dá
// para derivar de tax_law_chunks.
type DocMeta struct {
	ID          string
	Label       string
	SourceURL   string
	PublishedAt string // ISO (YYYY-MM-DD)
}

// catalog é indexado por metadata.source (o mesmo valor que
// ingestion.DocumentProfile.SourceLabel grava em cada chunk).
var catalog = map[string]DocMeta{
	"LC 68/2024": {
		ID:          "lc68-2024",
		Label:       "LC 68/2024",
		SourceURL:   "https://www.camara.leg.br/proposicoesWeb/fichadetramitacao?idProposicao=2430143",
		PublishedAt: "2024-07-22",
	},
	"LC 214/2025": {
		ID:          "lc214-2025",
		Label:       "LC 214/2025",
		SourceURL:   "https://www.planalto.gov.br/ccivil_03/leis/lcp/lcp214.htm",
		PublishedAt: "2025-01-16",
	},
}

// Document é um item de corpus_view — dado real (Version, ChunkPrefix,
// PublishedAt quando derivável) mesclado com o catálogo (Label, SourceURL,
// fallback de PublishedAt).
type Document struct {
	ID          string
	Label       string
	Version     string
	PublishedAt string
	SourceURL   string
	ChunkPrefix string
}

// ChangelogEntry é uma linha factual do changelog — nunca prosa inventada.
type ChangelogEntry struct {
	Type  string // sempre "rule": é um fato de indexação, não uma mudança anunciada de comportamento de IA
	Label string
	Desc  string
}

// View é o resultado completo — mapeia 1:1 para o payload de GET /law/corpus.
type View struct {
	Documents         []Document
	CurrentDocumentID string
	Changelog         []ChangelogEntry
}

var reISODatePrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

// publishedAtFromLeiPDFVersion extrai "YYYY-MM-DD" do prefixo de
// lei_pdf_version (ex.: "2024-07-22-doc-plp-68" -> "2024-07-22"). Devolve ""
// se o valor não começar com uma data nesse formato.
func publishedAtFromLeiPDFVersion(v string) string {
	return reISODatePrefix.FindString(v)
}

func formatDateBR(iso string) string {
	parts := strings.Split(iso, "-")
	if len(parts) != 3 {
		return ""
	}
	return parts[2] + "/" + parts[1] + "/" + parts[0]
}

// Build mescla as linhas do banco com o catálogo estático. currentSourceOverride
// (env LAW_CORPUS_CURRENT_SOURCE, lido pelo handler — este pacote não lê env)
// escolhe o documento corrente por metadata.source exato; vazio = o de mais
// chunks. Slices sempre não-nil (documents:[] / changelog:[] no corpus vazio
// — um slice nil serializa "null" em JSON e derruba o frontend).
func Build(rows []ingestion.CorpusDocumentRow, currentSourceOverride string) View {
	documents := make([]Document, 0, len(rows))
	changelog := make([]ChangelogEntry, 0, len(rows))

	override := strings.TrimSpace(currentSourceOverride)
	var currentID string
	var maxChunks int

	for _, r := range rows {
		if strings.TrimSpace(r.Source) == "" {
			continue // linha sem metadata.source — não deveria acontecer; não derruba a rota por isso
		}

		meta, known := catalog[r.Source]
		label := r.Source
		sourceURL := ""
		id := r.Source
		publishedAt := publishedAtFromLeiPDFVersion(r.LeiPDFVersion)
		if known {
			label = meta.Label
			sourceURL = meta.SourceURL
			id = meta.ID
			if publishedAt == "" {
				publishedAt = meta.PublishedAt
			}
		}

		documents = append(documents, Document{
			ID:          id,
			Label:       label,
			Version:     r.StructureVersion,
			PublishedAt: publishedAt,
			SourceURL:   sourceURL,
			ChunkPrefix: r.ChunkPrefix,
		})

		desc := fmt.Sprintf("Corpus normativo indexado no TribIA: %d trechos.", r.Chunks)
		if dateBR := formatDateBR(publishedAt); dateBR != "" {
			desc = fmt.Sprintf("Corpus normativo indexado no TribIA: %d trechos, data-base %s.", r.Chunks, dateBR)
		}
		changelog = append(changelog, ChangelogEntry{Type: "rule", Label: label, Desc: desc})

		switch {
		case override != "":
			if r.Source == override {
				currentID = id
			}
		case r.Chunks > maxChunks:
			maxChunks = r.Chunks
			currentID = id
		}
	}

	return View{
		Documents:         documents,
		CurrentDocumentID: currentID,
		Changelog:         changelog,
	}
}
