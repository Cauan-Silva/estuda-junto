import { useEffect, useState } from 'react'
import { NOME_FASE, relogio } from '../servicos/formato.js'

const RAIO = 120
const CIRCUNFERENCIA = 2 * Math.PI * RAIO

/** Quanto falta, calculado com o relógio daqui corrigido pelo do servidor. */
export function restanteAgora(timer, diferenca) {
  if (!timer) return 0
  if (!timer.rodando) return timer.restanteMs
  return Math.max(0, timer.fimEm - (Date.now() + diferenca))
}

export function useRestante(timer, diferenca) {
  const [restante, setRestante] = useState(() => restanteAgora(timer, diferenca))
  useEffect(() => {
    setRestante(restanteAgora(timer, diferenca))
    if (!timer?.rodando) return
    const id = setInterval(() => setRestante(restanteAgora(timer, diferenca)), 250)
    return () => clearInterval(id)
  }, [timer, diferenca])
  return restante
}

export default function Timer({ timer, diferenca, podeControlar, enviar, compacto = false }) {
  const restante = useRestante(timer, diferenca)
  if (!timer) return <div className="timer timer--carregando" aria-busy="true" />

  const progresso = timer.duracaoMs ? 1 - restante / timer.duracaoMs : 0
  const acao = (a) => enviar({ tipo: 'timer', acao: a })
  const pontos = Array.from({ length: timer.ciclosAteLonga }, (_, i) => i < timer.ciclo)

  return (
    <section className={`timer timer--${timer.fase} ${compacto ? 'timer--compacto' : ''}`} aria-label="Relógio da sala">
      <div className="timer__mostrador">
        <svg viewBox="0 0 280 280" className="timer__anel" aria-hidden="true">
          <circle cx="140" cy="140" r={RAIO} className="timer__trilha" />
          <circle
            cx="140"
            cy="140"
            r={RAIO}
            className="timer__progresso"
            strokeDasharray={CIRCUNFERENCIA}
            strokeDashoffset={CIRCUNFERENCIA * (1 - Math.min(1, Math.max(0, progresso)))}
          />
        </svg>
        <div className="timer__centro">
          <span className="timer__fase">{NOME_FASE[timer.fase]}</span>
          <span className="timer__tempo" role="timer" aria-live="off">
            {relogio(restante)}
          </span>
          <span className="timer__situacao">{timer.rodando ? 'contando' : restante === timer.duracaoMs ? 'pronto para começar' : 'pausado'}</span>
        </div>
      </div>

      <div className="timer__ciclos" aria-label={`${timer.ciclo} de ${timer.ciclosAteLonga} focos até a pausa longa`}>
        {pontos.map((feito, i) => (
          <span key={i} className={feito ? 'feito' : ''} />
        ))}
      </div>

      {podeControlar ? (
        <div className="timer__controles">
          {timer.rodando ? (
            <button type="button" className="botao botao--principal botao--grande" onClick={() => acao('pausar')}>
              Pausar
            </button>
          ) : (
            <button type="button" className="botao botao--principal botao--grande" onClick={() => acao('iniciar')}>
              {restante === timer.duracaoMs ? 'Começar' : 'Continuar'}
            </button>
          )}
          <button type="button" className="botao" onClick={() => acao('pular')} title="Termina esta fase e vai para a próxima">
            Pular fase
          </button>
          <button type="button" className="botao-link" onClick={() => acao('reiniciar')}>
            Reiniciar
          </button>
        </div>
      ) : (
        <p className="timer__aviso">Quem criou a sala controla o relógio.</p>
      )}
    </section>
  )
}
