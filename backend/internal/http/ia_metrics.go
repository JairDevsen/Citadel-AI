package http

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// Apenas metadados operacionais. O Master não recebe OCR, campos, nomes de arquivos ou dados de trabalhadores.
func (s *Server) adminIAMetrics(c *gin.Context) {
	dias, err := strconv.Atoi(c.DefaultQuery("dias", "30"))
	if err != nil || dias < 1 || dias > 365 {
		c.JSON(400, gin.H{"error": "dias deve estar entre 1 e 365"})
		return
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		c.JSON(400, gin.H{"error": "offset inválido"})
		return
	}
	s.sysJSON(c, iaMetricsSQL, dias, c.Query("tenant"), offset)
}

const iaMetricsSQL = `WITH lotes AS (
 SELECT b.*, coalesce(t.nome_fantasia,t.razao_social) AS empresa FROM batches b JOIN tenants t ON t.id=b.tenant_id
 WHERE b.criado_em >= now()-make_interval(days => $1) AND ($2::text='' OR b.tenant_id::text=$2)
), pg AS (
 SELECT p.*, coalesce((analise->>'tokens')::bigint,0) AS tokens,
 CASE WHEN coalesce((analise->>'tokens')::bigint,0)=0 THEN 0 ELSE (analise->'uso'->>'custo_estimado_brl')::numeric END AS custo,
 coalesce((analise->'uso'->>'custo_conhecido_brl')::numeric,0) AS custo_conhecido
 FROM pages p JOIN lotes b ON b.id=p.batch_id
), paginas AS (
 SELECT batch_id, count(*) AS regioes, count(DISTINCT coalesce(pagina_origem,numero)) AS folhas,
 count(*) FILTER(WHERE recorte <> '[0,0,1,1]'::jsonb) AS recortes,
 count(*) FILTER(WHERE proc_status='OK') AS analisadas,
 count(*) FILTER(WHERE proc_status='FALHOU') AS falhas,
 count(*) FILTER(WHERE analise->>'pagina_em_branco'='true') AS brancas,
 count(*) FILTER(WHERE analise->>'pagina_ilegivel'='true') AS ilegiveis,
 count(*) FILTER(WHERE analise->>'multiplos_documentos_na_imagem'='true') AS multi_pendentes,
 count(*) FILTER(WHERE analise->>'camada' LIKE 'Camada 0%') AS cache,
 sum(tokens) AS tokens, sum(custo_conhecido) AS custo_conhecido_brl,
 count(*) FILTER(WHERE custo IS NULL) AS paginas_sem_preco,
 CASE WHEN count(*) FILTER(WHERE custo IS NULL)=0 THEN sum(custo) END AS custo_estimado_brl,
 sum((analise->>'confianca')::numeric) FILTER(WHERE proc_status='OK' AND coalesce(analise->>'pagina_em_branco','false')<>'true') AS soma_confianca,
 count(*) FILTER(WHERE proc_status='OK' AND analise ? 'confianca' AND coalesce(analise->>'pagina_em_branco','false')<>'true') AS n_confianca
 FROM pg GROUP BY batch_id
), revisoes AS (
 SELECT d.batch_id, d.id, d.status, d.confianca, d.motivos_revisao, d.checklist,
 primeiro.antes, ultimo.depois,
 EXISTS(SELECT 1 FROM reviews r WHERE r.document_id=d.id AND r.acao IN ('DIVIDIR','JUNTAR')) AS separacao_corrigida,
 EXISTS(SELECT 1 FROM reviews r WHERE r.document_id=d.id AND r.acao='TRANSCRICAO_MANUAL') AS transcricao
 FROM documents d JOIN lotes b ON b.id=d.batch_id
 LEFT JOIN LATERAL (SELECT antes FROM reviews r WHERE r.document_id=d.id ORDER BY criado_em, id LIMIT 1) primeiro ON true
 LEFT JOIN LATERAL (SELECT depois FROM reviews r WHERE r.document_id=d.id ORDER BY criado_em DESC, id DESC LIMIT 1) ultimo ON true
 WHERE d.origem='IA' AND NOT d.motivos_revisao ? 'DIVISAO_MANUAL'
), docs AS (
 SELECT batch_id, count(*) AS documentos,
 count(*) FILTER(WHERE status='AGUARDANDO_REVISAO') AS aguardando_revisao,
 count(*) FILTER(WHERE status='REJEITADO') AS rejeitados,
 count(*) FILTER(WHERE separacao_corrigida) AS separacoes_corrigidas,
 count(*) FILTER(WHERE status='REVISADO' AND antes IS NOT NULL AND depois IS NOT NULL) AS revisados,
 count(*) FILTER(WHERE status='REVISADO' AND antes->>'Tipo'=depois->>'Tipo') AS tipos_corretos,
 count(*) FILTER(WHERE status='REVISADO' AND antes->'Campos'=depois->'Campos' AND antes->>'Tipo'=depois->>'Tipo'
   AND antes->'Paginas'=depois->'Paginas' AND (antes->'Competencia') IS NOT DISTINCT FROM (depois->'Competencia') AND NOT separacao_corrigida AND NOT transcricao) AS leituras_sem_correcao,
 count(*) FILTER(WHERE EXISTS (SELECT 1 FROM jsonb_array_elements(checklist) ck WHERE ck->>'regra'='Assinatura identificada' AND ck->>'status'='REVISAO_HUMANA')) AS assinaturas_pendentes
 FROM revisoes GROUP BY batch_id
), metricas AS (
 SELECT b.id, b.tenant_id, b.empresa, b.criado_em, b.status, b.tentativas,
 round(extract(epoch FROM coalesce(b.concluido_em,now())-b.iniciado_em)::numeric,1) AS duracao_seg,
 coalesce(p.folhas,0) AS folhas, coalesce(p.regioes,0) AS regioes, coalesce(p.analisadas,0) AS analisadas,
 coalesce(p.recortes,0) AS recortes, coalesce(p.falhas,0) AS falhas, coalesce(p.brancas,0) AS brancas,
 coalesce(p.ilegiveis,0) AS ilegiveis, coalesce(p.multi_pendentes,0) AS multi_pendentes, coalesce(p.cache,0) AS cache,
 coalesce(p.tokens,0) AS tokens, coalesce(p.custo_conhecido_brl,0) AS custo_conhecido_brl,
 coalesce(p.paginas_sem_preco,0) AS paginas_sem_preco, p.custo_estimado_brl,
 round(p.tokens::numeric/nullif(p.folhas,0),2) AS tokens_por_folha,
 round(p.tokens::numeric/nullif(p.analisadas,0),2) AS tokens_por_regiao,
 round(p.custo_estimado_brl/nullif(p.folhas,0),6) AS custo_por_folha_brl,
 coalesce(p.soma_confianca,0) AS soma_confianca, coalesce(p.n_confianca,0) AS n_confianca,
 round(100*p.soma_confianca/nullif(p.n_confianca,0),1) AS confianca_media_pct,
 coalesce(d.documentos,0) AS documentos, coalesce(d.aguardando_revisao,0) AS aguardando_revisao,
 coalesce(d.rejeitados,0) AS rejeitados, coalesce(d.separacoes_corrigidas,0) AS separacoes_corrigidas,
 coalesce(d.revisados,0) AS revisados, coalesce(d.tipos_corretos,0) AS tipos_corretos,
 coalesce(d.leituras_sem_correcao,0) AS leituras_sem_correcao, coalesce(d.assinaturas_pendentes,0) AS assinaturas_pendentes,
 round(100.0*d.tipos_corretos/nullif(d.revisados,0),1) AS acuracia_classificacao_pct,
 round(100.0*d.leituras_sem_correcao/nullif(d.revisados,0),1) AS leitura_sem_correcao_pct,
 round(100.0*d.revisados/nullif(d.documentos,0),1) AS cobertura_revisao_pct
 FROM lotes b LEFT JOIN paginas p ON p.batch_id=b.id LEFT JOIN docs d ON d.batch_id=b.id
), resumo AS (
 SELECT count(*) AS lotes, coalesce(sum(folhas),0) AS folhas, coalesce(sum(regioes),0) AS regioes,
 coalesce(sum(tokens),0) AS tokens, coalesce(sum(recortes),0) AS recortes,
 coalesce(sum(documentos),0) AS documentos, coalesce(sum(revisados),0) AS revisados,
 coalesce(sum(aguardando_revisao),0) AS aguardando_revisao, coalesce(sum(ilegiveis),0) AS ilegiveis,
 coalesce(sum(assinaturas_pendentes),0) AS assinaturas_pendentes, coalesce(sum(multi_pendentes),0) AS multi_pendentes,
 coalesce(sum(cache),0) AS cache, coalesce(sum(paginas_sem_preco),0) AS paginas_sem_preco,
 coalesce(sum(custo_conhecido_brl),0) AS custo_conhecido_brl,
 CASE WHEN coalesce(sum(paginas_sem_preco),0)=0 THEN coalesce(sum(custo_estimado_brl),0) END AS custo_estimado_brl,
 round(sum(tokens)::numeric/nullif(sum(folhas),0),2) AS tokens_por_folha,
 round(sum(tokens)::numeric/nullif(sum(analisadas),0),2) AS tokens_por_regiao,
 round(100*sum(soma_confianca)/nullif(sum(n_confianca),0),1) AS confianca_media_pct,
 round(100.0*sum(tipos_corretos)/nullif(sum(revisados),0),1) AS acuracia_classificacao_pct,
 round(100.0*sum(leituras_sem_correcao)/nullif(sum(revisados),0),1) AS leitura_sem_correcao_pct,
 round(100.0*sum(revisados)/nullif(sum(documentos),0),1) AS cobertura_revisao_pct
 FROM metricas
)
SELECT json_build_object('resumo',(SELECT row_to_json(r) FROM resumo r),
 'lotes',(SELECT coalesce(json_agg(x),'[]') FROM (SELECT * FROM metricas ORDER BY criado_em DESC,id LIMIT 50 OFFSET $3) x),
 'motivos',(SELECT coalesce(json_agg(x),'[]') FROM (SELECT motivo, count(*) AS quantidade FROM revisoes d CROSS JOIN LATERAL jsonb_array_elements_text(d.motivos_revisao) motivo GROUP BY motivo ORDER BY quantidade DESC LIMIT 12) x),
 'camadas',(SELECT coalesce(json_agg(x ORDER BY camada),'[]') FROM (
 SELECT coalesce(left(analise->>'camada',8),'Sem camada') AS camada, count(*) AS paginas,
 sum(tokens) AS tokens, round(avg(tokens),2) AS tokens_por_pagina,
 count(*) FILTER (WHERE analise->'desempenho' ? 'total_ms') AS paginas_com_tempo,
 round(avg((analise->'desempenho'->>'local_ms')::numeric),2) AS local_ms,
 round(avg((analise->'desempenho'->>'fila_ms')::numeric),2) AS fila_ms,
 round(avg((analise->'desempenho'->>'modelo_ms')::numeric),2) AS modelo_ms,
 round(avg((analise->'desempenho'->>'total_ms')::numeric),2) AS total_ms,
 round((percentile_cont(0.95) WITHIN GROUP (ORDER BY (analise->'desempenho'->>'total_ms')::double precision))::numeric,2) AS p95_ms,
 count(*) FILTER (WHERE analise->'desempenho'->>'texto_nativo_reusado'='true') AS texto_nativo_reusado
 FROM pg WHERE proc_status='OK' GROUP BY 1) x),
 'modelos',(SELECT coalesce(json_agg(x),'[]') FROM (SELECT u->>'modelo' AS modelo,count(*) AS chamadas, sum((u->>'tokens')::bigint) AS tokens,
 sum((u->>'entrada')::bigint) AS entrada, sum((u->>'saida')::bigint) AS saida, sum((u->>'cache')::bigint) AS cache,
 CASE WHEN count(*) FILTER(WHERE u->>'custo_estimado_brl' IS NULL)=0 THEN sum((u->>'custo_estimado_brl')::numeric) END AS custo_estimado_brl
 FROM pg CROSS JOIN LATERAL jsonb_array_elements(coalesce(analise->'uso'->'chamadas','[]'::jsonb)) u GROUP BY 1 ORDER BY tokens DESC) x))`
