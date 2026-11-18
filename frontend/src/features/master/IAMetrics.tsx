import { useState } from 'react'
import { Button, Card, DataTable, EmptyState, ErrorState, Field, KpiCard, Select, SkeletonRows, StatusPill } from '@/components/ui'
import { useGet } from '@/lib/query'
import { fmtDateTime } from '@/lib/format'

type Metrics = {
  lotes: number; folhas: number; regioes: number; tokens: number; recortes: number; documentos: number;
  revisados: number; aguardando_revisao: number; ilegiveis: number; assinaturas_pendentes: number;
  multi_pendentes: number; cache: number; paginas_sem_preco: number; custo_conhecido_brl: number;
  custo_estimado_brl: number | null; tokens_por_folha: number | null; tokens_por_regiao: number | null;
  confianca_media_pct: number | null; acuracia_classificacao_pct: number | null;
  leitura_sem_correcao_pct: number | null; cobertura_revisao_pct: number | null;
}
type Batch = Metrics & { id: string; empresa: string; criado_em: string; status: string; duracao_seg: number | null; tentativas: number; falhas: number; separacoes_corrigidas: number; custo_por_folha_brl: number | null }
type Layer = { camada: string; paginas: number; tokens: number; tokens_por_pagina: number; paginas_com_tempo: number; local_ms: number | null; fila_ms: number | null; modelo_ms: number | null; total_ms: number | null; p95_ms: number | null; texto_nativo_reusado: number }
type Response = { camadas?: Layer[]; resumo: Metrics; lotes: Batch[]; motivos: { motivo: string; quantidade: number }[]; modelos: { modelo: string; chamadas: number; tokens: number; entrada: number; saida: number; cache: number; custo_estimado_brl: number | null }[] }
export const metricNumber = (v: number | null | undefined) => v == null ? '—' : v.toLocaleString('pt-BR', { maximumFractionDigits: 2 })
export const metricPercent = (v: number | null | undefined) => v == null ? 'Sem amostra' : `${metricNumber(v)}%`
export const metricCost = (v: number | null | undefined) => v == null ? 'Sem tarifa' : v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 4, maximumFractionDigits: 6 })

export function IAMetrics({ tenantId, compact = false }: { tenantId?: string; compact?: boolean }) {
  const [dias, setDias] = useState('30')
  const [tenant, setTenant] = useState('')
  const [offset, setOffset] = useState(0)
  const [selected, setSelected] = useState<string | null>(null)
  const r = useGet<Response>('/admin/ia/metrics', { dias, tenant: tenantId ?? tenant, offset }, { refetchInterval: 30000 })
  const tenants = useGet<{ id: string; razao_social: string; nome_fantasia?: string }[]>(!tenantId && !compact ? '/admin/tenants' : null)
  if (r.error) return <ErrorState error={r.error} retry={r.refetch} />
  const m = r.data?.resumo
  const batch = r.data?.lotes.find(b => b.id === selected)
  return <section className="space-y-4" aria-label="Consumo e qualidade documental">
    <div className="flex flex-wrap items-end gap-4">
      <div className="mr-auto"><h2 className="font-display text-xl font-bold text-brand-darker">Consumo e qualidade documental</h2><p className="text-sm text-muted-foreground">Lotes recebidos no período · atualização a cada 30 segundos</p></div>
      <Field label="Período">{id => <Select id={id} value={dias} onChange={e => { setDias(e.target.value); setOffset(0); setSelected(null) }}><option value="7">Últimos 7 dias</option><option value="30">Últimos 30 dias</option><option value="90">Últimos 90 dias</option><option value="365">Últimos 365 dias</option></Select>}</Field>
      {!tenantId && !compact && <Field label="Empresa">{id => <Select id={id} value={tenant} onChange={e => { setTenant(e.target.value); setOffset(0); setSelected(null) }}><option value="">Todas as empresas</option>{tenants.data?.map(t => <option key={t.id} value={t.id}>{t.nome_fantasia ?? t.razao_social}</option>)}</Select>}</Field>}
    </div>
    {!m ? <SkeletonRows /> : <>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <KpiCard label="Tokens registrados" value={metricNumber(m.tokens)} hint={`${m.folhas} folhas · ${m.regioes} regiões · ${m.cache} reutilizações`} />
        <KpiCard label="Média de tokens por folha" value={metricNumber(m.tokens_por_folha)} hint={`${metricNumber(m.tokens_por_regiao)} por região analisada`} />
        <KpiCard label="Custo estimado" value={metricCost(m.custo_estimado_brl)} hint={m.paginas_sem_preco ? `${m.paginas_sem_preco} regiões sem tarifa · parcial ${metricCost(m.custo_conhecido_brl)}` : 'Tarifas configuradas por modelo em BRL'} />
        <KpiCard label="Acurácia da classificação" value={metricPercent(m.acuracia_classificacao_pct)} hint={`${m.revisados} documentos revisados · cobertura ${metricPercent(m.cobertura_revisao_pct)}`} />
      </div>
      {!compact && <>
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <KpiCard label="Leitura sem correção" value={metricPercent(m.leitura_sem_correcao_pct)} hint="Tipo, campos e páginas iguais após revisão" />
          <KpiCard label="Confiança média da IA" value={metricPercent(m.confianca_media_pct)} hint="Estimativa do motor; não mede acurácia" />
          <KpiCard label="Recortes automáticos" value={m.recortes} hint={`${m.multi_pendentes} regiões ainda com múltiplos documentos`} />
          <KpiCard label="Aguardando revisão" value={m.aguardando_revisao} hint={`${m.ilegiveis} regiões ilegíveis · ${m.assinaturas_pendentes} assinaturas não verificadas`} />
        </div>
        <p className="text-sm text-muted-foreground">A acurácia compara a classificação inicial com a revisão humana concluída. A amostra revisada pode concentrar casos difíceis e não representa todo o lote. “Leitura sem correção” compara campos estruturados, tipo e separação; não mede cada caractere do OCR. Sem revisão, o indicador fica sem amostra. Consumo contabiliza respostas registradas, incluindo alternativas de modelo; falhas sem resposta de uso e chamadas interrompidas podem não constar. O custo é uma estimativa, não a fatura do provedor.</p>
        <Card title="Desempenho por camada final">
          <p className="mb-3 text-sm text-muted-foreground">Tempo por região: download, leitura local, espera pelo worker e modelos. Não equivale à duração total do lote e não inclui a espera de cópias por uma leitura compartilhada. A camada final inclui o consumo das tentativas anteriores. Tempos históricos sem medição ficam em branco; reutilização de cache não herda a duração da leitura original.</p>
          <DataTable rows={r.data?.camadas} empty={<EmptyState title="Sem análises concluídas" />} cols={[
            { key: 'camada', header: 'Camada' }, { key: 'paginas', header: 'Regiões' },
            { key: 'tokens_por_pagina', header: 'Tokens / região', render: x => metricNumber(x.tokens_por_pagina) },
            { key: 'local_ms', header: 'Local (ms)', render: x => metricNumber(x.local_ms) },
            { key: 'fila_ms', header: 'Fila (ms)', render: x => metricNumber(x.fila_ms) },
            { key: 'modelo_ms', header: 'Modelos (ms)', render: x => metricNumber(x.modelo_ms) },
            { key: 'total_ms', header: 'Média (ms)', render: x => metricNumber(x.total_ms) },
            { key: 'p95_ms', header: 'P95 (ms)', render: x => metricNumber(x.p95_ms) },
            { key: 'paginas_com_tempo', header: 'Amostra de tempos' },
            { key: 'texto_nativo_reusado', header: 'Texto nativo reutilizado' },
          ]} />
        </Card>
        <Card title="Indicadores por lote">
          <DataTable rows={r.data?.lotes} onRow={b => setSelected(b.id)} empty={<EmptyState title="Nenhum lote no período" />} cols={[
            { key: 'id', header: 'Lote', render: b => <span title={b.id}>{b.id.slice(0, 8)}<span className="block text-xs text-muted-foreground">{fmtDateTime(b.criado_em)}</span></span> },
            { key: 'empresa', header: 'Empresa' }, { key: 'status', header: 'Status', render: b => <StatusPill status={b.status} /> },
            { key: 'folhas', header: 'Folhas / regiões', render: b => `${b.folhas} / ${b.regioes}` },
            { key: 'tokens', header: 'Tokens', render: b => metricNumber(b.tokens) },
            { key: 'tokens_por_folha', header: 'Tokens / folha', render: b => metricNumber(b.tokens_por_folha) },
            { key: 'custo_estimado_brl', header: 'Custo estimado', render: b => metricCost(b.custo_estimado_brl) },
            { key: 'acuracia_classificacao_pct', header: 'Acurácia / amostra', render: b => `${metricPercent(b.acuracia_classificacao_pct)} (${b.revisados})` },
          ]} />
          <div className="mt-3 flex items-center justify-between gap-3"><Button disabled={offset === 0} onClick={() => { setOffset(Math.max(0, offset-50)); setSelected(null) }}>Anteriores</Button><span className="text-sm">{m.lotes ? `${offset+1}–${Math.min(offset+50, m.lotes)} de ${m.lotes}` : '0 lotes'} · selecione um lote para detalhar</span><Button disabled={offset+50 >= m.lotes} onClick={() => { setOffset(offset+50); setSelected(null) }}>Próximos</Button></div>
        </Card>
        {batch && <Card title={`Detalhes do lote ${batch.id.slice(0, 8)}`}><dl className="grid gap-3 text-sm sm:grid-cols-3">
          {Object.entries({ 'Documentos': batch.documentos, 'Recortes': batch.recortes, 'Tempo decorrido': `${metricNumber(batch.duracao_seg)} s`, 'Tentativas adicionais': batch.tentativas, 'Regiões com falha': batch.falhas, 'Regiões ilegíveis': batch.ilegiveis, 'Assinaturas não verificadas': batch.assinaturas_pendentes, 'Separações corrigidas': batch.separacoes_corrigidas, 'Leitura sem correção': metricPercent(batch.leitura_sem_correcao_pct), 'Confiança da IA': metricPercent(batch.confianca_media_pct), 'Custo por folha': metricCost(batch.custo_por_folha_brl), 'Regiões sem tarifa': batch.paginas_sem_preco }).map(([k,v]) => <div key={k}><dt className="text-muted-foreground">{k}</dt><dd className="font-semibold">{v}</dd></div>)}
        </dl></Card>}
        <div className="grid gap-4 xl:grid-cols-2">
          <Card title="Uso por modelo"><DataTable rows={r.data?.modelos} empty={<EmptyState title="Sem chamadas externas registradas" />} cols={[{ key: 'modelo', header: 'Modelo' }, { key: 'chamadas', header: 'Chamadas' }, { key: 'entrada', header: 'Entrada' }, { key: 'saida', header: 'Saída' }, { key: 'cache', header: 'Cache' }, { key: 'custo_estimado_brl', header: 'Estimativa', render: v => metricCost(v.custo_estimado_brl) }]} /></Card>
          <Card title="Motivos de revisão mais frequentes"><DataTable rows={r.data?.motivos} empty={<EmptyState title="Sem motivos de revisão" />} cols={[{ key: 'motivo', header: 'Motivo' }, { key: 'quantidade', header: 'Documentos' }]} /></Card>
        </div>
      </>}
    </>}
  </section>
}
