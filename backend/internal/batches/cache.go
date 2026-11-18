package batches

// Consulta apenas os hashes pendentes do lote. Cada lookup usa o índice tenant/hash/motor,
// em vez de ordenar todas as páginas já analisadas da empresa.
const reusePagesSQL = `WITH targets AS MATERIALIZED (
 SELECT DISTINCT hash_sha256, texto_nativo FROM pages
 WHERE batch_id=$1 AND tenant_id=$4 AND proc_status<>'OK' AND hash_sha256 IS NOT NULL
), src AS MATERIALIZED (
 SELECT t.hash_sha256, t.texto_nativo, s.analise, s.batch_id FROM targets t
 CROSS JOIN LATERAL (
   SELECT p.analise, p.batch_id FROM pages p
   WHERE p.tenant_id=$4 AND p.hash_sha256=t.hash_sha256 AND p.texto_nativo=t.texto_nativo
     AND p.proc_status='OK' AND p.analise IS NOT NULL AND p.hash_sha256 IS NOT NULL
     AND p.analise->>'motor'=$3 AND p.analise->>'tipo_lote_contexto'=$5 AND p.batch_id<>$1
   ORDER BY p.processado_em DESC, p.id LIMIT 1
 ) s
), upd AS (
 UPDATE pages p SET proc_status='OK', erro=NULL, processado_em=now(),
 analise = (src.analise - 'desempenho') || jsonb_build_object(
   'camada',$2::text, 'tokens',0, 'reutilizada_de',src.batch_id::text,
   'desempenho',jsonb_build_object('reutilizada',true),
   'uso',jsonb_build_object('chamadas','[]'::jsonb,'tokens_entrada',0,'tokens_saida',0,
     'tokens_cache',0,'custo_estimado_brl',0,'custo_conhecido_brl',0,'chamadas_sem_preco',0))
 FROM src WHERE p.batch_id=$1 AND p.tenant_id=$4 AND p.proc_status<>'OK'
   AND p.hash_sha256=src.hash_sha256 AND p.texto_nativo=src.texto_nativo RETURNING 1
) SELECT count(*) FROM upd`
