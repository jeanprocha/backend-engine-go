package waitlist

import "testing"

func TestNormalizeEmail(t *testing.T) {
	cases := map[string]string{
		"  Nome@Ex.com  ": "nome@ex.com",
		"ja@minusculo.com": "ja@minusculo.com",
		"":                 "",
	}
	for in, want := range cases {
		if got := NormalizeEmail(in); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateForInsert(t *testing.T) {
	valid := []string{"a@b.com", "  Nome@Ex.COM  ", "consultor+tribia@empresa.com.br"}
	for _, e := range valid {
		if err := ValidateForInsert(e); err != nil {
			t.Errorf("ValidateForInsert(%q) devia ser válido, deu erro: %v", e, err)
		}
	}

	invalid := []string{"", "   ", "nao-e-email", "@sem-usuario.com", "sem-arroba.com"}
	for _, e := range invalid {
		if err := ValidateForInsert(e); err == nil {
			t.Errorf("ValidateForInsert(%q) devia falhar, não deu erro", e)
		}
	}
}

func TestValidateForInsert_ExcedeTamanho(t *testing.T) {
	local := ""
	for i := 0; i < 250; i++ {
		local += "a"
	}
	tooLong := local + "@x.com"
	if err := ValidateForInsert(tooLong); err == nil {
		t.Errorf("ValidateForInsert com e-mail de %d chars devia falhar", len(tooLong))
	}
}
