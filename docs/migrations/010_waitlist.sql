-- Lista de espera capturada pela landing (Etapa M/PR 9) — substitui o CTA
-- morto ("Entrar na lista de espera") por um formulário real.
-- e-mail normalizado (minúsculas, sem espaço) antes de gravar — a unicidade
-- é sobre esse valor normalizado, não o texto bruto que o usuário digitou.

CREATE TABLE IF NOT EXISTS waitlist (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(254) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT waitlist_email_unique UNIQUE (email)
);

CREATE INDEX IF NOT EXISTS idx_waitlist_created_at ON waitlist (created_at DESC);
