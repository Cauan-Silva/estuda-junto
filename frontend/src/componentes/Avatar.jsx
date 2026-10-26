import { iniciais } from '../servicos/formato.js'

// Cada pessoa ganha uma das cores de marca-texto, sempre a mesma (pelo id).
const CORES = ['foco', 'pausa', 'pausa_longa', 'verde', 'laranja']

export default function Avatar({ nome, id, tamanho = 32, children }) {
  const cor = CORES[Math.abs(Number(id) || 0) % CORES.length]
  return (
    <span
      className={`avatar avatar--${cor}`}
      style={{ width: tamanho, height: tamanho, fontSize: tamanho * 0.4 }}
      aria-hidden="true"
    >
      {iniciais(nome)}
      {children}
    </span>
  )
}
