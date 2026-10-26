import { useState } from 'react'

export default function Tarefas({ tarefas, enviar }) {
  const [texto, setTexto] = useState('')
  const pendentes = tarefas.filter((t) => !t.feita).length

  function adicionar(e) {
    e.preventDefault()
    if (texto.trim() && enviar({ tipo: 'tarefa_nova', texto })) setTexto('')
  }

  return (
    <div className="tarefas">
      <form className="tarefas__nova" onSubmit={adicionar}>
        <input value={texto} onChange={(e) => setTexto(e.target.value)} maxLength={200} placeholder="Nova tarefa da turma" aria-label="Nova tarefa" />
        <button className="botao" disabled={!texto.trim()}>
          Adicionar
        </button>
      </form>
      {tarefas.length === 0 ? (
        <p className="vazio">O quadro está vazio. Anote o que a turma vai estudar hoje.</p>
      ) : (
        <>
          <p className="tarefas__resumo">
            {pendentes === 0 ? 'Tudo feito. Boa!' : `${pendentes} ${pendentes === 1 ? 'pendente' : 'pendentes'} de ${tarefas.length}`}
          </p>
          <ul className="tarefas__lista">
            {tarefas.map((t) => (
              <li key={t.id} className={t.feita ? 'tarefa tarefa--feita' : 'tarefa'}>
                <label>
                  <input type="checkbox" checked={t.feita} onChange={(e) => enviar({ tipo: 'tarefa_marcar', id: t.id, feita: e.target.checked })} />
                  <span className="tarefa__texto">{t.texto}</span>
                </label>
                <span className="tarefa__quem">{t.feita && t.feitaPor ? `feita por ${t.feitaPor}` : t.autorNome}</span>
                <button type="button" className="tarefa__apagar" onClick={() => enviar({ tipo: 'tarefa_apagar', id: t.id })} aria-label={`Apagar "${t.texto}"`}>
                  ×
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  )
}
