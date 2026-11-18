DROP INDEX reviews_document_time;
ALTER TABLE pages DROP COLUMN pagina_origem, DROP COLUMN regiao, DROP COLUMN recorte, DROP COLUMN rotacao_origem;

ALTER TABLE batches DROP COLUMN total_folhas;
