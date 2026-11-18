import { useEffect, useRef, useState } from 'react'
import { AlertCircle, RotateCcw, Send, ShieldCheck, X } from 'lucide-react'
import { clsx } from '@/components/ui'
import { ApiError, api } from '@/lib/api'
import type { ChatApiResponse, ChatMessage } from './types'

const sugestoes = [
  'Resumo geral de compliance',
  'Documentos vencidos ou a vencer',
  'Pendências de colaboradores',
  'Riscos abertos no momento',
]

function SentinelAvatar({ iconClassName }: { iconClassName: string }) {
  const [falhou, setFalhou] = useState(false)

  if (falhou) return <ShieldCheck className={iconClassName} aria-hidden />

  return (
    <img
      src="/sentinel-avatar.png"
      alt=""
      className="h-full w-full scale-[1.8] object-cover"
      onError={() => setFalhou(true)}
    />
  )
}

function mensagemErro(err: unknown) {
  if (err instanceof ApiError) return err.message
  if (err instanceof Error) return err.message
  return 'Não foi possível consultar o assistente. Tente novamente.'
}

export function ChatWidget() {
  const [aberto, setAberto] = useState(false)
  const [mensagens, setMensagens] = useState<ChatMessage[]>([])
  const [texto, setTexto] = useState('')
  const [carregando, setCarregando] = useState(false)
  const [erro, setErro] = useState<string | null>(null)
  const listaRef = useRef<HTMLDivElement>(null)
  const enviandoRef = useRef(false)

  useEffect(() => {
    if (!aberto) return
    const fecharComEsc = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setAberto(false)
    }
    document.addEventListener('keydown', fecharComEsc)
    return () => document.removeEventListener('keydown', fecharComEsc)
  }, [aberto])

  useEffect(() => {
    if (aberto) listaRef.current?.scrollTo({ top: listaRef.current.scrollHeight, behavior: 'smooth' })
  }, [aberto, carregando, mensagens])

  async function enviar(pergunta: string) {
    const conteudo = pergunta.trim()
    if (!conteudo || enviandoRef.current) return

    enviandoRef.current = true
    const mensagemUsuario: ChatMessage = {
      id: crypto.randomUUID(),
      role: 'user',
      content: conteudo,
      timestamp: new Date().toISOString(),
    }
    const historico = [...mensagens, mensagemUsuario]
    setMensagens(historico)
    setTexto('')
    setErro(null)
    setCarregando(true)

    try {
      const resposta = await api<ChatApiResponse>('/chat', {
        method: 'POST',
        body: {
          mensagens: historico.slice(-10).map(({ role, content }) => ({ role, content })),
        },
      })
      setMensagens((atuais) => [...atuais, {
        id: crypto.randomUUID(),
        role: 'assistant',
        content: resposta.resposta,
        fontes: resposta.fontes,
        timestamp: new Date().toISOString(),
      }])
    } catch (err) {
      setErro(mensagemErro(err))
    } finally {
      enviandoRef.current = false
      setCarregando(false)
    }
  }

  function limparConversa() {
    if (carregando) return
    setMensagens([])
    setErro(null)
    setTexto('')
  }

  return (
    <>
      {!aberto && (
        <button
          type="button"
          className="fixed bottom-5 right-5 z-30 grid h-14 w-14 place-items-center overflow-hidden rounded-full border border-brand-light/80 bg-brand-lighter text-brand-default shadow-lg transition hover:scale-105 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-default"
          aria-label="Abrir Citadel Sentinel"
          onClick={() => setAberto(true)}
        >
          <SentinelAvatar iconClassName="h-7 w-7 text-brand-default" />
        </button>
      )}

      {aberto && (
        <div className="fixed inset-0 z-40">
          <button
            type="button"
            className="absolute inset-0 bg-black/30 backdrop-blur-[1px]"
            aria-label="Fechar assistente"
            onClick={() => setAberto(false)}
          />
          <section
            className="fixed inset-y-0 right-0 z-40 flex w-full flex-col border-l border-border bg-surface shadow-2xl sm:w-[420px]"
            role="dialog"
            aria-modal="true"
            aria-labelledby="chat-titulo"
          >
            <header className="flex items-center gap-3 border-b border-border bg-brand-50/70 px-4 py-4">
              <span className="grid h-10 w-10 shrink-0 place-items-center overflow-hidden rounded-full border border-brand-light/70 bg-brand-lighter text-brand-default">
                <SentinelAvatar iconClassName="h-5 w-5 text-brand-default" />
              </span>
              <div className="min-w-0 flex-1">
                <h2 id="chat-titulo" className="font-semibold text-brand-darker">Citadel Sentinel</h2>
                <p className="text-xs text-muted-foreground">Assistente Consultivo de Compliance</p>
              </div>
              <button
                type="button"
                className="icon-circle"
                aria-label="Limpar conversa"
                title="Limpar conversa"
                disabled={carregando}
                onClick={limparConversa}
              >
                <RotateCcw className="h-4 w-4" aria-hidden />
              </button>
              <button
                type="button"
                className="icon-circle"
                aria-label="Fechar assistente"
                onClick={() => setAberto(false)}
              >
                <X className="h-4 w-4" aria-hidden />
              </button>
            </header>

            <div ref={listaRef} className="flex-1 space-y-4 overflow-y-auto p-4" aria-live="polite">
              {mensagens.length === 0 ? (
                <div className="flex min-h-full flex-col justify-center">
                  <div className="mb-5 text-center">
                    <ShieldCheck className="mx-auto mb-2 h-8 w-8 text-brand-default" aria-hidden />
                    <p className="text-sm font-medium text-brand-darker">Olá, sou o Sentinel. Como posso ajudar?</p>
                    <p className="mt-1 text-xs text-muted-foreground">Posso consultar informações da sua empresa em modo somente leitura.</p>
                  </div>
                  <div className="flex flex-wrap justify-center gap-2">
                    {sugestoes.map((sugestao) => (
                      <button
                        key={sugestao}
                        type="button"
                        className="max-w-full rounded-full border border-brand-light/70 bg-brand-lighter/70 px-3.5 py-2 text-left text-xs font-medium leading-snug text-brand-darker transition hover:border-brand-default hover:bg-brand-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-default sm:text-sm"
                        onClick={() => {
                          setTexto(sugestao)
                          void enviar(sugestao)
                        }}
                      >
                        {sugestao}
                      </button>
                    ))}
                  </div>
                </div>
              ) : (
                mensagens.map((mensagem) => (
                  <article
                    key={mensagem.id}
                    className={clsx('max-w-[90%] rounded-2xl px-3.5 py-3 text-sm', mensagem.role === 'user'
                      ? 'ml-auto bg-brand-default text-white'
                      : 'mr-auto border border-border bg-brand-50 text-brand-darker')}
                  >
                    <p className="whitespace-pre-wrap break-words">{mensagem.content}</p>
                    {mensagem.role === 'assistant' && !!mensagem.fontes?.length && (
                      <div className="mt-2 flex flex-wrap gap-1.5 border-t border-border/60 pt-2">
                        {mensagem.fontes.map((fonte) => (
                          <span key={fonte} className="rounded-full bg-surface px-2 py-0.5 text-[10px] text-muted-foreground">
                            {fonte}
                          </span>
                        ))}
                      </div>
                    )}
                  </article>
                ))
              )}
              {carregando && (
                <p className="mr-auto flex items-center gap-2 rounded-2xl border border-border bg-brand-50 px-3.5 py-3 text-sm text-muted-foreground">
                  <span className="h-2 w-2 animate-pulse rounded-full bg-brand-default" aria-hidden />
                  Consultando dados da empresa...
                </p>
              )}
              {erro && (
                <p role="alert" className="flex items-start gap-2 rounded-lg border border-danger/30 bg-danger-bg/60 p-3 text-sm text-danger">
                  <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
                  <span>{erro}</span>
                </p>
              )}
            </div>

            <form
              className="border-t border-border bg-surface p-3"
              onSubmit={(event) => {
                event.preventDefault()
                void enviar(texto)
              }}
            >
              <div className="flex items-end gap-2">
                <textarea
                  className="field-cristal min-h-11 max-h-32 flex-1 resize-y"
                  aria-label="Sua pergunta"
                  placeholder="Escreva sua pergunta..."
                  value={texto}
                  rows={1}
                  maxLength={4000}
                  disabled={carregando}
                  onChange={(event) => setTexto(event.target.value)}
                />
                <button
                  type="submit"
                  className="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-brand-default text-white transition hover:bg-brand-darker disabled:cursor-not-allowed disabled:opacity-50"
                  aria-label="Enviar pergunta"
                  disabled={carregando || !texto.trim()}
                >
                  <Send className="h-4 w-4" aria-hidden />
                </button>
              </div>
              <p className="mt-2 text-center text-[11px] text-muted-foreground">Assistente consultivo · Não realiza alterações</p>
            </form>
          </section>
        </div>
      )}
    </>
  )
}
