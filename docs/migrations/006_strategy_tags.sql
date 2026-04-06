-- Tags de contexto para chips na UI (heurística; não substitui classificação fiscal).
-- pattern armazenado em minúsculas; unicidade global.

CREATE TABLE IF NOT EXISTS strategy_tags (
    id BIGSERIAL PRIMARY KEY,
    pattern VARCHAR(100) NOT NULL,
    label VARCHAR(100) NOT NULL,
    category VARCHAR(50),
    color_scheme VARCHAR(20) NOT NULL DEFAULT 'emerald',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT strategy_tags_pattern_unique UNIQUE (pattern)
);

CREATE INDEX IF NOT EXISTS idx_strategy_tags_pattern ON strategy_tags (pattern);

-- Seed alinhado a frontend-next/src/constants/strategy-mapping.ts (+ engenharia para liberal)
INSERT INTO strategy_tags (pattern, label, category, color_scheme) VALUES
    ('saas', 'Modelo Digital Detectado', 'Perfil', 'blue'),
    ('software', 'Modelo Digital Detectado', 'Perfil', 'blue'),
    ('licenciamento', 'Modelo Digital Detectado', 'Perfil', 'blue'),
    ('assinatura', 'Modelo Digital Detectado', 'Perfil', 'blue'),
    ('exportação', 'Imunidade Art. 52 (Export)', 'Imunidade', 'emerald'),
    ('exterior', 'Imunidade Art. 52 (Export)', 'Imunidade', 'emerald'),
    ('exportar', 'Imunidade Art. 52 (Export)', 'Imunidade', 'emerald'),
    ('venda fora', 'Imunidade Art. 52 (Export)', 'Imunidade', 'emerald'),
    ('imobiliário', 'Regime Específico Imobiliário', 'Setor', 'amber'),
    ('aluguel', 'Regime Específico Imobiliário', 'Setor', 'amber'),
    ('incorporação', 'Regime Específico Imobiliário', 'Setor', 'amber'),
    ('venda de imóvel', 'Regime Específico Imobiliário', 'Setor', 'amber'),
    ('advogado', 'Redutor de Alíquota (30%)', 'Profissional', 'purple'),
    ('médico', 'Redutor de Alíquota (30%)', 'Profissional', 'purple'),
    ('engenheiro', 'Redutor de Alíquota (30%)', 'Profissional', 'purple'),
    ('engenharia', 'Redutor de Alíquota (30%)', 'Profissional', 'purple'),
    ('profissional liberal', 'Redutor de Alíquota (30%)', 'Profissional', 'purple')
ON CONFLICT (pattern) DO NOTHING;
