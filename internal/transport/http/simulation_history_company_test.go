package http

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jeanprocha/backend-engine-go/internal/history"
)

func TestParseOptionalCompanyID(t *testing.T) {
	validID := uuid.New()
	validUpper := validID.String()

	cases := []struct {
		name    string
		raw     *string
		wantNil bool
		wantErr bool
		wantVal string
	}{
		{name: "nil pointer", raw: nil, wantNil: true},
		{name: "empty string", raw: strPtr(""), wantNil: true},
		{name: "só espaços", raw: strPtr("   "), wantNil: true},
		{name: "UUID válido normaliza", raw: strPtr("  " + validUpper + "  "), wantVal: validID.String()},
		{name: "lixo é erro", raw: strPtr("nao-e-um-uuid"), wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseOptionalCompanyID(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("esperava erro, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if tc.wantNil {
				if got != nil {
					t.Fatalf("esperava nil, got %q", *got)
				}
				return
			}
			if got == nil || *got != tc.wantVal {
				t.Fatalf("esperava %q, got %v", tc.wantVal, got)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

func TestSimulationRecordDetailFromHistory_companyID(t *testing.T) {
	base := history.Detail{
		ID:             uuid.New(),
		CreatedAt:      time.Now(),
		Year:           2026,
		CompanyContext: "Empresa de teste",
	}

	t.Run("company_id presente propaga para o DTO", func(t *testing.T) {
		id := uuid.New().String()
		d := base
		d.CompanyID = &id

		resp := simulationRecordDetailFromHistory(&d)
		if resp.CompanyID == nil || *resp.CompanyID != id {
			t.Fatalf("esperava company_id %q, got %v", id, resp.CompanyID)
		}
	})

	t.Run("company_id ausente (registo legado) fica nil no DTO", func(t *testing.T) {
		d := base
		d.CompanyID = nil

		resp := simulationRecordDetailFromHistory(&d)
		if resp.CompanyID != nil {
			t.Fatalf("esperava nil, got %q", *resp.CompanyID)
		}
	})
}
