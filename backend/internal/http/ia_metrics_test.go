package http

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Run against a disposable database with all migrations applied.
func TestIAMetricsUsesPagesAndReviewedSample(t *testing.T) {
	url := os.Getenv("METRICS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("METRICS_TEST_DATABASE_URL not configured")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`SELECT set_config('app.bypass','on',true)`)
	mkTenant := func(cnpj string) string {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO tenants(cnpj,razao_social,porte,quota_bytes,quota_paginas_mes,dek_wrapped) VALUES($1,'Metrics test','PEQUENA',10000,1000,'\x00') RETURNING id::text`, cnpj).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	mkBatch := func(tenant string) string {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO batches(tenant_id,arquivo_original,storage_key,sha256,bytes,status,total_paginas) VALUES($1,'private-file.pdf','test','test',1,'PROCESSADO',3) RETURNING id::text`, tenant).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b := mkTenant("metrics-a"), mkTenant("metrics-b")
	ba, bb := mkBatch(a), mkBatch(b)
	exec(`INSERT INTO pages(tenant_id,batch_id,numero,pagina_origem,regiao,recorte,storage_key,proc_status,analise) VALUES
    ($1,$2,1,1,1,'[0,0,1,0.5]','test','OK','{"tokens":100,"confianca":0.8,"uso":{"custo_estimado_brl":0.0001,"custo_conhecido_brl":0.0001}}'),
    ($1,$2,2,1,2,'[0,0.5,1,1]','test','OK','{"tokens":50,"confianca":0.9,"uso":{"custo_estimado_brl":0.0002,"custo_conhecido_brl":0.0002}}'),
    ($1,$2,3,2,1,'[0,0,1,1]','test','OK','{"tokens":0,"pagina_em_branco":true,"camada":"Camada 0: Cache"}')`, a, ba)
	exec(`INSERT INTO pages(tenant_id,batch_id,numero,pagina_origem,storage_key,proc_status,analise) VALUES($1,$2,1,1,'test','OK','{"tokens":700}')`, b, bb)
	exec(`UPDATE pages SET analise=analise || jsonb_build_object('camada','Camada 3: Visão','desempenho',
        jsonb_build_object('total_ms', CASE WHEN numero=1 THEN 50 ELSE 150 END,
          'local_ms',10,'modelo_ms',CASE WHEN numero=1 THEN 40 ELSE 140 END,
          'texto_nativo_reusado',numero=1)) WHERE batch_id=$1 AND numero IN (1,2)`, ba)
	exec(`INSERT INTO document_types(codigo,nome,categoria) VALUES('METRIC_TEST','Metrics','OUTROS')`)
	for i := 0; i < 2; i++ {
		var d string
		if err := tx.QueryRow(ctx, `INSERT INTO documents(tenant_id,batch_id,document_type,paginas,status,finops) VALUES($1,$2,'METRIC_TEST','{1,2}','REVISADO','{"tokens_consumidos":99999}') RETURNING id::text`, a, ba).Scan(&d); err != nil {
			t.Fatal(err)
		}
		initial := `{"Tipo":"METRIC_TEST","Campos":{"nome":"PRIVATE"},"Paginas":[1,2]}`
		if i == 0 {
			initial = `{"Tipo":"OUTRO","Campos":{"nome":"WRONG"},"Paginas":[1,2]}`
		}
		exec(`INSERT INTO reviews(tenant_id,document_id,acao,antes,depois,criado_em) VALUES($1,$2,'CORRIGIR',$3,'{"Tipo":"METRIC_TEST","Campos":{"nome":"PRIVATE"},"Paginas":[1,2]}',now()-interval '1 minute')`, a, d, initial)
		exec(`INSERT INTO reviews(tenant_id,document_id,acao,antes,depois) VALUES($1,$2,'CONFIRMAR','{"Tipo":"METRIC_TEST","Campos":{"nome":"PRIVATE"},"Paginas":[1,2]}','{"Tipo":"METRIC_TEST","Campos":{"nome":"PRIVATE"},"Paginas":[1,2]}')`, a, d)
	}
	get := func(tenant string) map[string]any {
		t.Helper()
		var raw []byte
		if err := tx.QueryRow(ctx, iaMetricsSQL, 30, tenant, 0).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := get(a)
	r := out["resumo"].(map[string]any)
	for k, want := range map[string]float64{"folhas": 2, "regioes": 3, "recortes": 2, "tokens": 150, "tokens_por_folha": 75, "tokens_por_regiao": 50, "revisados": 2, "acuracia_classificacao_pct": 50, "leitura_sem_correcao_pct": 50, "custo_estimado_brl": 0.0003} {
		if r[k] != want {
			t.Errorf("%s = %v, want %v", k, r[k], want)
		}
	}
	camadas := out["camadas"].([]any)
	if len(camadas) != 2 {
		t.Fatalf("camadas = %v", camadas)
	}
	cache, visao := camadas[0].(map[string]any), camadas[1].(map[string]any)
	if cache["total_ms"] != nil || cache["p95_ms"] != nil || cache["paginas_com_tempo"] != float64(0) {
		t.Fatalf("cache sem medição deve ter tempo nulo: %v", cache)
	}
	for k, want := range map[string]float64{"paginas": 2, "tokens_por_pagina": 75, "paginas_com_tempo": 2, "total_ms": 100, "p95_ms": 145, "local_ms": 10, "modelo_ms": 90, "texto_nativo_reusado": 1} {
		if visao[k] != want {
			t.Errorf("camada 3 %s = %v, want %v", k, visao[k], want)
		}
	}
	r = get(b)["resumo"].(map[string]any)
	if r["custo_estimado_brl"] != nil || r["acuracia_classificacao_pct"] != nil || r["paginas_sem_preco"] != float64(1) {
		t.Fatalf("missing prices/sample: %v", r)
	}
	r = get("")["resumo"].(map[string]any)
	if r["tokens"] != float64(850) || r["custo_estimado_brl"] != nil {
		t.Fatalf("global totals: %v", r)
	}
}
