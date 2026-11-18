-- Dados demonstrativos idempotentes de compliance para Agrotech.
-- Execute no banco local de desenvolvimento; não remove nem altera colaboradores.
BEGIN;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM users
    WHERE lower(email) = 'admin@agrotech.demo'
      AND tenant_id IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'tenant de admin@agrotech.demo não encontrado';
  END IF;
END $$;

WITH tenant AS (
  SELECT tenant_id AS id
  FROM users
  WHERE lower(email) = 'admin@agrotech.demo'
    AND tenant_id IS NOT NULL
), docs AS (
  SELECT id AS tenant_id,
         md5(id::text || ':demo-compliance:pgr')::uuid AS id,
         'PROGRAMA_SST'::text AS document_type,
         'PGR Anual - Programa de Gerenciamento de Riscos'::text AS titulo,
         current_date - 377 AS data_documento,
         current_date - 12 AS validade_ate,
         'REVISADO'::text AS status,
         jsonb_build_object('origem_demo', true, 'situacao_compliance', 'VENCIDO', 'dias_vencido', 12) AS campos,
         jsonb_build_array(jsonb_build_object('severidade', 'erro', 'codigo', 'DOCUMENTO_VENCIDO', 'mensagem', 'Vencido há 12 dias')) AS alertas,
         NULL::text AS nome_arquivo
  FROM tenant
  UNION ALL
  SELECT id,
         md5(id::text || ':demo-compliance:pcmso')::uuid,
         'PROGRAMA_SST',
         'PCMSO - Programa de Controle Médico de Saúde Ocupacional',
         current_date - 350,
         current_date + 15,
         'AGUARDANDO_REVISAO',
         jsonb_build_object('origem_demo', true, 'situacao_compliance', 'A_VENCER', 'dias_para_vencer', 15),
         jsonb_build_array(jsonb_build_object('severidade', 'aviso', 'codigo', 'DOCUMENTO_A_VENCER', 'mensagem', 'Vence em 15 dias')),
         NULL::text
  FROM tenant
  UNION ALL
  SELECT id,
         md5(id::text || ':demo-compliance:alvara')::uuid,
         'ALVARA_LICENCA',
         'Alvará de Funcionamento - Matriz Garanhuns',
         current_date - 90,
         current_date + 275,
         'AUTO_APROVADO',
         jsonb_build_object('origem_demo', true, 'situacao_compliance', 'OK', 'unidade', 'Matriz Garanhuns'),
         '[]'::jsonb,
         NULL::text
  FROM tenant
  UNION ALL
  SELECT id,
         md5(id::text || ':demo-compliance:licenca-ambiental')::uuid,
         'ALVARA_LICENCA',
         'Licença Ambiental - Unidade Garanhuns',
         current_date - 165,
         current_date + 200,
         'REVISADO',
         jsonb_build_object('origem_demo', true, 'situacao_compliance', 'OK', 'unidade', 'Unidade Garanhuns'),
         '[]'::jsonb,
         NULL::text
  FROM tenant
)
INSERT INTO documents (
  id, tenant_id, document_type, titulo, data_documento, validade_ate,
  paginas, confianca, status, campos, alertas, texto_anonimizado,
  nome_arquivo, pasta_destino, origem
)
SELECT id, tenant_id, document_type, titulo, data_documento, validade_ate,
       '{}'::integer[], 0.98, status, campos, alertas,
       'Documento fictício de demonstração de compliance da Agrotech.',
       nome_arquivo, 'Compliance/Corporativos', 'MANUAL'
FROM docs
ON CONFLICT (id) DO UPDATE SET
  document_type = EXCLUDED.document_type,
  titulo = EXCLUDED.titulo,
  data_documento = EXCLUDED.data_documento,
  validade_ate = EXCLUDED.validade_ate,
  status = EXCLUDED.status,
  campos = EXCLUDED.campos,
  alertas = EXCLUDED.alertas,
  texto_anonimizado = EXCLUDED.texto_anonimizado,
  nome_arquivo = EXCLUDED.nome_arquivo,
  pasta_destino = EXCLUDED.pasta_destino,
  origem = EXCLUDED.origem,
  atualizado_em = now()
WHERE documents.tenant_id = EXCLUDED.tenant_id;

WITH tenant AS (
  SELECT tenant_id AS id
  FROM users
  WHERE lower(email) = 'admin@agrotech.demo'
    AND tenant_id IS NOT NULL
), carlos AS (
  SELECT id
  FROM employees
  WHERE tenant_id = (SELECT id FROM tenant)
    AND upper(nome) = 'CARLOS EDUARDO SILVA'
  ORDER BY id
  LIMIT 1
), risk_seed AS (
  SELECT 'ASO_VENCIDO'::text AS codigo,
         'ALTO'::text AS severidade,
         'SEGURANCA'::text AS categoria,
         'ASO periódico pendente para operação de maquinário'::text AS titulo,
         'Registro demonstrativo: confirmar a realização e a validade do exame ocupacional periódico antes da atividade com maquinário.'::text AS descricao,
         'NR-7 (PCMSO)'::text AS base_legal,
         'DEMO-ASO-PERIODICO-2026'::text AS referencia
  UNION ALL
  SELECT 'NR_VENCIDO',
         'MEDIO',
         'SEGURANCA',
         'Certificado NR-31 pendente',
         'Registro demonstrativo: confirmar o certificado e a capacitação aplicável às atividades rurais e à operação de equipamentos.' ,
         'NR-31',
         'DEMO-NR31-2026'
)
INSERT INTO risk_indicators (
  id, tenant_id, employee_id, supplier_id, document_id, codigo,
  severidade, categoria, titulo, descricao, base_legal, referencia, status
)
SELECT md5(tenant.id::text || ':demo-compliance:risk:' || risk_seed.codigo)::uuid,
       tenant.id, carlos.id, NULL, NULL, risk_seed.codigo,
       risk_seed.severidade, risk_seed.categoria, risk_seed.titulo,
       risk_seed.descricao, risk_seed.base_legal, risk_seed.referencia, 'ABERTO'
FROM tenant
CROSS JOIN risk_seed
LEFT JOIN carlos ON true
ON CONFLICT (
  tenant_id,
  coalesce(employee_id, '00000000-0000-0000-0000-000000000000'::uuid),
  coalesce(supplier_id, '00000000-0000-0000-0000-000000000000'::uuid),
  coalesce(document_id, '00000000-0000-0000-0000-000000000000'::uuid),
  codigo,
  referencia
)
DO UPDATE SET
  severidade = EXCLUDED.severidade,
  categoria = EXCLUDED.categoria,
  titulo = EXCLUDED.titulo,
  descricao = EXCLUDED.descricao,
  base_legal = EXCLUDED.base_legal,
  status = 'ABERTO',
  justificativa = NULL,
  atualizado_em = now();

COMMIT;

SELECT d.titulo, d.status, d.data_documento, d.validade_ate,
       CASE
         WHEN d.validade_ate < current_date THEN 'VENCIDO'
         WHEN d.validade_ate <= current_date + 30 THEN 'A_VENCER'
         ELSE 'OK'
       END AS situacao_compliance
FROM documents d
WHERE EXISTS (
  SELECT 1 FROM users u
  WHERE u.tenant_id = d.tenant_id AND lower(u.email) = 'admin@agrotech.demo'
)
  AND d.id IN (
    md5(d.tenant_id::text || ':demo-compliance:pgr')::uuid,
    md5(d.tenant_id::text || ':demo-compliance:pcmso')::uuid,
    md5(d.tenant_id::text || ':demo-compliance:alvara')::uuid,
    md5(d.tenant_id::text || ':demo-compliance:licenca-ambiental')::uuid
  )
ORDER BY d.titulo;

SELECT ri.titulo, ri.severidade, ri.status, e.nome AS colaborador
FROM risk_indicators ri
LEFT JOIN employees e ON e.id = ri.employee_id
WHERE EXISTS (
  SELECT 1 FROM users u
  WHERE u.tenant_id = ri.tenant_id AND lower(u.email) = 'admin@agrotech.demo'
)
  AND ri.referencia IN ('DEMO-ASO-PERIODICO-2026', 'DEMO-NR31-2026')
ORDER BY ri.titulo;
