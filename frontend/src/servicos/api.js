// Chamadas à API. O front-end e a API ficam no mesmo endereço (em desenvolvimento, o Vite
// repassa /api para o Go), então os caminhos são relativos.

const CHAVE_TOKEN = 'estuda-junto.token'

export function lerToken() {
  try {
    return localStorage.getItem(CHAVE_TOKEN)
  } catch {
    return null
  }
}

export function guardarToken(token) {
  try {
    if (token) localStorage.setItem(CHAVE_TOKEN, token)
    else localStorage.removeItem(CHAVE_TOKEN)
  } catch {
    // navegação anônima sem armazenamento: o login vale só nesta aba
  }
}

export class ErroApi extends Error {
  constructor(mensagem, status) {
    super(mensagem)
    this.status = status
  }
}

async function pedir(metodo, caminho, corpo) {
  const token = lerToken()
  let resposta
  try {
    resposta = await fetch(caminho, {
      method: metodo,
      headers: {
        ...(corpo ? { 'Content-Type': 'application/json' } : {}),
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: corpo ? JSON.stringify(corpo) : undefined,
    })
  } catch {
    throw new ErroApi('Sem conexão com o servidor. Confira sua internet.', 0)
  }
  const dados = await resposta.json().catch(() => ({}))
  if (!resposta.ok) {
    if (resposta.status === 401 && token) window.dispatchEvent(new Event('sessao-expirada'))
    throw new ErroApi(dados.erro || 'Algo deu errado. Tente de novo.', resposta.status)
  }
  return dados
}

export const api = {
  cadastrar: (nome, email, senha) => pedir('POST', '/api/auth/cadastro', { nome, email, senha }),
  entrar: (email, senha) => pedir('POST', '/api/auth/login', { email, senha }),
  eu: () => pedir('GET', '/api/eu'),
  salas: () => pedir('GET', '/api/salas'),
  criarSala: (nome, descricao, publica) => pedir('POST', '/api/salas', { nome, descricao, publica }),
  sala: (codigo) => pedir('GET', `/api/salas/${encodeURIComponent(codigo)}`),
  ranking: (periodo) => pedir('GET', `/api/ranking?periodo=${periodo}`),
  rankingSala: (codigo, periodo) => pedir('GET', `/api/salas/${encodeURIComponent(codigo)}/ranking?periodo=${periodo}`),
  rtc: () => pedir('GET', '/api/rtc'),
}

export function enderecoWebSocket(codigo) {
  const protocolo = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocolo}//${location.host}/ws/salas/${encodeURIComponent(codigo)}?token=${encodeURIComponent(lerToken() || '')}`
}
