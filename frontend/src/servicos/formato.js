/** 1500 segundos → "25 min"; 5400 → "1 h 30 min". */
export function duracao(segundos) {
  const total = Math.round((segundos || 0) / 60)
  if (total < 60) return `${total} min`
  const h = Math.floor(total / 60)
  const m = total % 60
  return m ? `${h} h ${m} min` : `${h} h`
}

/** Milissegundos → "24:59" (ou "1:04:59" acima de uma hora). */
export function relogio(ms) {
  const s = Math.max(0, Math.ceil(ms / 1000))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const seg = String(s % 60).padStart(2, '0')
  return h ? `${h}:${String(m).padStart(2, '0')}:${seg}` : `${String(m).padStart(2, '0')}:${seg}`
}

export const NOME_FASE = { foco: 'Foco', pausa: 'Pausa', pausa_longa: 'Pausa longa' }

const horaCurta = new Intl.DateTimeFormat('pt-BR', { hour: '2-digit', minute: '2-digit' })
export const hora = (iso) => horaCurta.format(new Date(iso))

const diaSemana = new Intl.DateTimeFormat('pt-BR', { weekday: 'short' })
export const diaCurto = (data) => diaSemana.format(new Date(`${data}T12:00:00`)).replace('.', '')

export function iniciais(nome) {
  const partes = (nome || '?').trim().split(/\s+/)
  return ((partes[0]?.[0] || '') + (partes.length > 1 ? partes.at(-1)[0] : '')).toUpperCase()
}
