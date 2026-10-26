import { useEffect, useState } from 'react'
import { duracao } from '../servicos/formato.js'

const PERIODOS = [
  ['semana', 'Semana'],
  ['mes', 'Mês'],
  ['sempre', 'Sempre'],
]

/** Ranking de horas de foco. "buscar" recebe o período e devolve {posicoes}. */
export default function Ranking({ titulo, idTitulo, buscar, destaque, atualizarCada }) {
  const [periodo, setPeriodo] = useState('semana')
  const [posicoes, setPosicoes] = useState(null)
  const [erro, setErro] = useState(null)

  useEffect(() => {
    let ativo = true
    const carregar = () =>
      buscar(periodo)
        .then((r) => ativo && (setPosicoes(r.posicoes), setErro(null)))
        .catch((e) => ativo && setErro(e.message))
    carregar()
    const intervalo = atualizarCada ? setInterval(carregar, atualizarCada) : null
    return () => {
      ativo = false
      clearInterval(intervalo)
    }
  }, [buscar, periodo, atualizarCada])

  const maior = posicoes?.[0]?.segundos || 1

  return (
    <div className="ranking">
      <div className="titulo-secao">
        <h2 id={idTitulo}>{titulo}</h2>
        <div className="segmentos" role="group" aria-label="Período">
          {PERIODOS.map(([valor, nome]) => (
            <button key={valor} type="button" aria-pressed={periodo === valor} onClick={() => setPeriodo(valor)}>
              {nome}
            </button>
          ))}
        </div>
      </div>
      {erro && <p className="erro">{erro}</p>}
      {!posicoes && !erro && <div className="esqueleto" style={{ height: 120 }} />}
      {posicoes?.length === 0 && <p className="vazio">Ninguém focou neste período ainda. O tempo conta quando o relógio roda na fase de foco.</p>}
      {posicoes?.length > 0 && (
        <ol className="ranking__lista">
          {posicoes.map((p, i) => (
            <li key={p.usuarioId} className={p.usuarioId === destaque ? 'ranking__voce' : undefined}>
              <span className="ranking__posicao">{i + 1}</span>
              <span className="ranking__nome">
                {p.nome}
                {p.usuarioId === destaque && <span className="sr"> (você)</span>}
              </span>
              <span className="ranking__tempo">{duracao(p.segundos)}</span>
              <span className="ranking__barra" style={{ width: `${(p.segundos / maior) * 100}%` }} aria-hidden="true" />
            </li>
          ))}
        </ol>
      )}
    </div>
  )
}
