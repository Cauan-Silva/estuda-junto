import { useEffect, useState } from 'react'
import Avatar from './Avatar.jsx'

export default function Pessoas({ membros, voce, donoId, enviar }) {
  const eu = membros.find((m) => m.usuarioId === voce?.usuarioId)
  const [status, setStatus] = useState(eu?.status || '')
  useEffect(() => setStatus(eu?.status || ''), [eu?.status])

  function salvar(e) {
    e.preventDefault()
    enviar({ tipo: 'status', texto: status })
  }

  return (
    <div className="pessoas">
      <form className="pessoas__status" onSubmit={salvar}>
        <label className="campo">
          <span>O que você está estudando?</span>
          <input
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            onBlur={() => status !== (eu?.status || '') && enviar({ tipo: 'status', texto: status })}
            maxLength={60}
            placeholder="Ex.: Lista de física, cap. 4"
          />
        </label>
      </form>
      <ul className="pessoas__lista">
        {membros.map((m) => (
          <li key={m.usuarioId} className="pessoa">
            <Avatar nome={m.nome} id={m.usuarioId} tamanho={36}>
              {m.focando && <span className="pessoa__foco" title="Focando agora" />}
            </Avatar>
            <span className="pessoa__textos">
              <strong>
                {m.nome}
                {m.usuarioId === voce?.usuarioId && ' (você)'}
              </strong>
              <span>{m.status || (m.focando ? 'Focando' : 'Na sala')}</span>
            </span>
            <span className="pessoa__selos">
              {m.usuarioId === donoId && <span className="selo">criou a sala</span>}
              {m.naChamada && <span className="selo selo--chamada">na chamada</span>}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
