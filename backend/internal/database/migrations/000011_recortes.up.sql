-- Página é a unidade analisada; pagina_origem identifica a folha física antes do recorte.
ALTER TABLE pages ADD COLUMN pagina_origem int,
  ADD COLUMN regiao int NOT NULL DEFAULT 1 CHECK (regiao > 0),
  ADD COLUMN recorte jsonb NOT NULL DEFAULT '[0,0,1,1]',
  ADD COLUMN rotacao_origem int NOT NULL DEFAULT 0;
UPDATE pages SET pagina_origem=numero;
ALTER TABLE pages ADD CONSTRAINT pages_origem_positive CHECK (pagina_origem > 0);
CREATE INDEX reviews_document_time ON reviews(document_id, criado_em, id);

ALTER TABLE batches ADD COLUMN total_folhas int NOT NULL DEFAULT 0 CHECK (total_folhas >= 0);
UPDATE batches SET total_folhas=total_paginas;
