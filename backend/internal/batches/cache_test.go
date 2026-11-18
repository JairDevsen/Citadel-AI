package batches

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestPageCacheScopesAndClearsUsage(t *testing.T) {
	url := os.Getenv("METRICS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("use disposable migrated database")
	}
	ctx := context.Background()
	c, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	tx, err := c.Begin(ctx)
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
	tenant := func(cnpj string) string {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO tenants(cnpj,razao_social,porte,quota_bytes,quota_paginas_mes,dek_wrapped) VALUES($1,'cache test','PEQUENA',10000,10000,'\x00') RETURNING id::text`, cnpj).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	batch := func(tenant, hash string) string {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO batches(tenant_id,arquivo_original,storage_key,sha256,bytes) VALUES($1,'test','test',$2,1) RETURNING id::text`, tenant, hash).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b := tenant("cache-a"), tenant("cache-b")
	source, target, foreign := batch(a, "source"), batch(a, "target"), batch(b, "foreign")
	add := func(tenant, batch string, n int, hash, motor, contexto, status string, nativo bool) {
		raw, _ := json.Marshal(map[string]any{"motor": motor, "tipo_lote_contexto": contexto, "tokens": 500, "uso": map[string]any{"custo_estimado_brl": .03}, "desempenho": map[string]any{"modelo_ms": 123}})
		exec(`INSERT INTO pages(tenant_id,batch_id,numero,pagina_origem,hash_sha256,storage_key,proc_status,analise,processado_em,texto_nativo) VALUES($1,$2,$3,$3,$4,'test',$5,$6,now(),$7)`, tenant, batch, n, hash, status, raw, nativo)
	}
	add(a, source, 1, "same", "v2", "MISTO", "OK", false)
	add(a, source, 2, "wrong-motor", "v1", "MISTO", "OK", false)
	add(a, source, 3, "wrong-context", "v2", "RH", "OK", false)
	add(b, foreign, 1, "foreign", "v2", "MISTO", "OK", false)
	for n, h := range []string{"same", "wrong-motor", "wrong-context", "foreign", "same", "same", "own"} {
		add(a, target, n+1, h, "", "", "PENDENTE", n == 4)
	}
	add(a, target, 8, "own", "v2", "MISTO", "OK", false)
	var count int
	if err := tx.QueryRow(ctx, reusePagesSQL, target, CamadaCache, "v2", a, "MISTO").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("reused %d, expected two identical pages from the same tenant/motor/context/text origin", count)
	}
	var tokens int
	var cost float64
	var perf json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT (analise->>'tokens')::int,(analise->'uso'->>'custo_estimado_brl')::numeric,analise->'desempenho' FROM pages WHERE batch_id=$1 AND numero=1`, target).Scan(&tokens, &cost, &perf); err != nil {
		t.Fatal(err)
	}
	if tokens != 0 || cost != 0 || string(perf) != `{"reutilizada": true}` {
		t.Fatalf("cached accounting %d %f %s", tokens, cost, perf)
	}
	if err := tx.QueryRow(ctx, `SELECT (analise->>'tokens')::int FROM pages WHERE batch_id=$1 AND numero=1`, source).Scan(&tokens); err != nil {
		t.Fatal(err)
	}
	if tokens != 500 {
		t.Fatal("source accounting mutated")
	}
	if err := tx.QueryRow(ctx, reusePagesSQL, target, CamadaCache, "v2", a, "MISTO").Scan(&count); err != nil || count != 0 {
		t.Fatalf("idempotence count=%d error=%v", count, err)
	}
}
