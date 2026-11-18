-- OCR reaproveitado não deve ser confundido com texto digital nativo.
ALTER TABLE pages ADD COLUMN texto_nativo boolean NOT NULL DEFAULT false;
UPDATE pages SET texto_nativo=true WHERE captura='pdf' AND texto_len+coalesce(length(texto_embutido),0)>0;
CREATE INDEX pages_cache_motor ON pages
  (tenant_id, hash_sha256, (analise->>'motor'), texto_nativo, processado_em DESC, id)
  WHERE proc_status='OK' AND analise IS NOT NULL AND hash_sha256 IS NOT NULL;
