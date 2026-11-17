export type ChatRole = 'user' | 'assistant'

export interface ChatMessage {
  id: string
  role: ChatRole
  content: string
  fontes?: string[]
  timestamp: string
}

export interface ChatApiResponse {
  resposta: string
  fontes?: string[]
  tokens?: number
}
