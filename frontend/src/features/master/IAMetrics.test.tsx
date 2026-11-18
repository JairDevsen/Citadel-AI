import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { IAMetrics, metricCost, metricPercent } from './IAMetrics'

vi.mock('@/lib/query', () => ({ useGet: (path: string) => path === '/admin/tenants' ? { data: [] } : {
  data: { resumo: { lotes: 0, folhas: 0, regioes: 0, tokens: 0, recortes: 0, documentos: 0, revisados: 0, aguardando_revisao: 0, ilegiveis: 0, assinaturas_pendentes: 0, multi_pendentes: 0, cache: 0, paginas_sem_preco: 1, custo_conhecido_brl: .0001, custo_estimado_brl: null, tokens_por_folha: null, tokens_por_regiao: null, confianca_media_pct: null, acuracia_classificacao_pct: null, leitura_sem_correcao_pct: null, cobertura_revisao_pct: null }, lotes: [], motivos: [], modelos: [] }, refetch: vi.fn(),
} }))

describe('Medição de consumo e qualidade', () => {
  it('distingue tarifa ausente, custo pequeno e amostra sem revisão', () => {
    expect(metricCost(null)).toBe('Sem tarifa')
    expect(metricCost(.000123)).toContain('0,000123')
    expect(metricPercent(null)).toBe('Sem amostra')
    expect(metricPercent(0)).toBe('0%')
  })
  it('explica os limites da medição e mostra o estado vazio sem inventar acurácia', () => {
    render(<IAMetrics />)
    expect(screen.getByText('Sem tarifa')).toBeInTheDocument()
    expect(screen.getByText('Nenhum lote no período')).toBeInTheDocument()
    expect(screen.getByText(/não mede cada caractere do OCR/)).toBeInTheDocument()
    expect(screen.queryByText('100%')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Próximos' })).toBeDisabled()
  })
})
