import { useEffect, useRef, useState } from 'react'

/** Janela com as durações do pomodoro (só para quem criou a sala). */
export default function ConfigSala({ aberta, aoFechar, timer, sala, enviar }) {
  const dialogo = useRef(null)
  const [valores, setValores] = useState(null)

  useEffect(() => {
    const d = dialogo.current
    if (!d) return
    if (aberta && !d.open) {
      setValores({
        focoMin: timer.focoMin,
        pausaMin: timer.pausaMin,
        pausaLongaMin: timer.pausaLongaMin,
        ciclosAteLonga: timer.ciclosAteLonga,
        todosControlam: sala.todosControlam,
      })
      d.showModal()
    } else if (!aberta && d.open) {
      d.close()
    }
  }, [aberta, timer, sala])

  const numero = (campo, min, max, rotulo, sufixo) => (
    <label className="campo campo--numero">
      <span>{rotulo}</span>
      <span className="campo__com-sufixo">
        <input
          type="number"
          min={min}
          max={max}
          required
          value={valores?.[campo] ?? ''}
          onChange={(e) => setValores((v) => ({ ...v, [campo]: e.target.value === '' ? '' : Number(e.target.value) }))}
        />
        <span>{sufixo}</span>
      </span>
    </label>
  )

  return (
    <dialog ref={dialogo} className="dialogo" onClose={aoFechar} aria-labelledby="config-titulo">
      <form
        className="formulario"
        onSubmit={(e) => {
          e.preventDefault()
          enviar({ tipo: 'config', config: valores })
          aoFechar()
        }}
      >
        <h2 id="config-titulo">Relógio da sala</h2>
        <div className="dialogo__grade">
          {numero('focoMin', 1, 180, 'Foco', 'min')}
          {numero('pausaMin', 1, 60, 'Pausa', 'min')}
          {numero('pausaLongaMin', 1, 90, 'Pausa longa', 'min')}
          {numero('ciclosAteLonga', 1, 12, 'Pausa longa a cada', 'focos')}
        </div>
        <label className="caixa">
          <input
            type="checkbox"
            checked={Boolean(valores?.todosControlam)}
            onChange={(e) => setValores((v) => ({ ...v, todosControlam: e.target.checked }))}
          />
          <span>Todos na sala podem começar, pausar e pular</span>
        </label>
        <p className="dica">Se o relógio estiver no meio de uma fase, as durações novas valem a partir da próxima.</p>
        <div className="dialogo__acoes">
          <button type="button" className="botao-link" onClick={aoFechar}>
            Cancelar
          </button>
          <button className="botao botao--principal">Salvar</button>
        </div>
      </form>
    </dialog>
  )
}
