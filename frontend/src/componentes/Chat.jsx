import { useEffect, useRef, useState } from 'react'
import { hora } from '../servicos/formato.js'

export default function Chat({ mensagens, voce, enviar, emFoco }) {
  const [texto, setTexto] = useState('')
  const lista = useRef(null)
  const noFim = useRef(true)

  // Desce sozinho para a mensagem nova, a não ser que a pessoa tenha subido para ler.
  useEffect(() => {
    const el = lista.current
    if (el && noFim.current) el.scrollTop = el.scrollHeight
  }, [mensagens])

  function mandar(e) {
    e.preventDefault()
    if (!texto.trim()) return
    if (enviar({ tipo: 'chat', texto })) setTexto('')
  }

  return (
    <div className="chat">
      <ol
        className="chat__mensagens"
        ref={lista}
        onScroll={(e) => {
          const el = e.currentTarget
          noFim.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
        }}
        aria-live="polite"
      >
        {mensagens.length === 0 && <li className="vazio">Nenhuma mensagem ainda. Diga oi para a turma.</li>}
        {mensagens.map((m, i) => {
          const minha = m.autorId === voce?.usuarioId
          const seguida = i > 0 && mensagens[i - 1].autorId === m.autorId
          return (
            <li key={m.id} className={`mensagem ${minha ? 'mensagem--minha' : ''} ${seguida ? 'mensagem--seguida' : ''}`}>
              {!seguida && (
                <span className="mensagem__autor">
                  {minha ? 'Você' : m.autorNome} <time dateTime={m.criadaEm}>{hora(m.criadaEm)}</time>
                </span>
              )}
              <p>{m.texto}</p>
            </li>
          )
        })}
      </ol>
      <form className="chat__envio" onSubmit={mandar}>
        <input
          value={texto}
          onChange={(e) => setTexto(e.target.value)}
          maxLength={500}
          placeholder={emFoco ? 'Estão em foco: deixe para a pausa, se der' : 'Escreva uma mensagem'}
          aria-label="Mensagem"
        />
        <button className="botao" disabled={!texto.trim()}>
          Enviar
        </button>
      </form>
    </div>
  )
}
