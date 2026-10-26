// Conexão com a sala por WebSocket: guarda o estado que o servidor manda e reconecta
// sozinha se a internet cair (recebendo o estado inteiro de novo).
import { useCallback, useEffect, useReducer, useRef } from 'react'
import { enderecoWebSocket } from './api.js'

const inicial = {
  conexao: 'conectando', // conectando | aberta | reconectando | fechada
  erroFatal: null,
  sala: null,
  voce: null,
  timer: null,
  diferencaRelogio: 0, // relógio do servidor - relógio daqui, em ms
  membros: [],
  tarefas: [],
  mensagens: [],
  chamada: [],
  limiteChamada: 6,
  aviso: null,
  ultimaFase: null,
}

function comRelogio(estado, timer) {
  // Corrige o relógio deste computador pelo do servidor (a metade da ida e volta é desprezível aqui).
  return timer ? { timer, diferencaRelogio: timer.agora - Date.now() } : {}
}

function reduzir(estado, acao) {
  switch (acao.tipo) {
    case 'conexao':
      return { ...estado, conexao: acao.valor }
    case 'fatal':
      return { ...estado, conexao: 'fechada', erroFatal: acao.valor }
    case 'limpar_aviso':
      return { ...estado, aviso: null }
    case 'mensagem': {
      const m = acao.valor
      switch (m.tipo) {
        case 'estado':
          return {
            ...estado,
            conexao: 'aberta',
            sala: m.sala,
            voce: m.voce,
            ...comRelogio(estado, m.timer),
            membros: m.membros ?? [],
            tarefas: m.tarefas ?? [],
            mensagens: m.mensagens ?? [],
            chamada: m.chamada ?? [],
            limiteChamada: m.limiteChamada ?? 6,
          }
        case 'timer':
          return { ...estado, ...comRelogio(estado, m.timer) }
        case 'fase':
          return { ...estado, ultimaFase: { terminou: m.terminou, comecou: m.comecou, em: Date.now() } }
        case 'membros':
          return { ...estado, membros: m.membros ?? [] }
        case 'tarefas':
          return { ...estado, tarefas: m.tarefas ?? [] }
        case 'chat':
          return { ...estado, mensagens: [...estado.mensagens, m.mensagem].slice(-100) }
        case 'sala':
          return { ...estado, sala: m.sala, voce: m.voce }
        case 'chamada':
          return { ...estado, chamada: m.chamada ?? [] }
        case 'erro':
          return { ...estado, aviso: { texto: m.erro, em: Date.now() } }
        default:
          return estado
      }
    }
    default:
      return estado
  }
}

export function useSala(codigo, { aoSinal } = {}) {
  const [estado, despachar] = useReducer(reduzir, inicial)
  const socketRef = useRef(null)
  const aoSinalRef = useRef(aoSinal)
  aoSinalRef.current = aoSinal

  useEffect(() => {
    let encerrado = false
    let tentativas = 0
    let espera = null

    function conectar() {
      const ws = new WebSocket(enderecoWebSocket(codigo))
      socketRef.current = ws
      ws.onopen = () => {
        tentativas = 0
      }
      ws.onmessage = (evento) => {
        let m
        try {
          m = JSON.parse(evento.data)
        } catch {
          return
        }
        if (m.tipo === 'sinal') aoSinalRef.current?.(m.de, m.dados)
        else despachar({ tipo: 'mensagem', valor: m })
      }
      ws.onclose = (evento) => {
        if (socketRef.current === ws) socketRef.current = null
        if (encerrado) return
        // A tela da sala confere antes (pela API) se a sala existe e se o login vale; aqui,
        // uma queda é internet ou o servidor gratuito acordando: tenta de novo, cada vez mais devagar.
        void evento
        tentativas++
        despachar({ tipo: 'conexao', valor: 'reconectando' })
        const atraso = Math.min(15000, 500 * 2 ** Math.min(tentativas, 5)) + Math.random() * 500
        espera = setTimeout(conectar, atraso)
      }
    }

    conectar()
    const aoVoltarOnline = () => {
      if (!socketRef.current) {
        clearTimeout(espera)
        conectar()
      }
    }
    window.addEventListener('online', aoVoltarOnline)
    return () => {
      encerrado = true
      clearTimeout(espera)
      window.removeEventListener('online', aoVoltarOnline)
      socketRef.current?.close()
    }
  }, [codigo])

  const enviar = useCallback((mensagem) => {
    const ws = socketRef.current
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(mensagem))
      return true
    }
    return false
  }, [])

  const limparAviso = useCallback(() => despachar({ tipo: 'limpar_aviso' }), [])
  const falhou = useCallback((texto) => despachar({ tipo: 'fatal', valor: texto }), [])

  return { ...estado, enviar, limparAviso, falhou }
}
