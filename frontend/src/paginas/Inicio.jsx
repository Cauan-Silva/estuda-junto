import { useCallback, useEffect, useState } from 'react'
import Cabecalho from '../componentes/Cabecalho.jsx'
import Ranking from '../componentes/Ranking.jsx'
import { api } from '../servicos/api.js'
import { diaCurto, duracao, NOME_FASE } from '../servicos/formato.js'
import { navegar } from '../servicos/rota.jsx'
import { useSessao } from '../servicos/sessao.jsx'

export default function Inicio() {
  const { usuario } = useSessao()
  const [salas, setSalas] = useState(null)
  const [estatisticas, setEstatisticas] = useState(null)
  const [erro, setErro] = useState(null)
  const [erroEstatisticas, setErroEstatisticas] = useState(false)

  const carregarEstatisticas = useCallback((tentativa = 0) => {
    setErroEstatisticas(false)
    api
      .eu()
      .then((r) => setEstatisticas(r.estatisticas))
      .catch(() => {
        // o banco gratuito pode estar acordando: tenta mais duas vezes antes de avisar
        if (tentativa < 2) setTimeout(() => carregarEstatisticas(tentativa + 1), 3000)
        else setErroEstatisticas(true)
      })
  }, [])

  const carregar = useCallback(() => {
    setErro(null)
    api.salas().then(setSalas).catch((e) => setErro(e.message))
    carregarEstatisticas()
  }, [carregarEstatisticas])

  useEffect(() => {
    carregar()
    const atualizar = setInterval(() => api.salas().then(setSalas).catch(() => {}), 20000)
    return () => clearInterval(atualizar)
  }, [carregar])

  return (
    <>
      <Cabecalho />
      <main className="inicio">
        <section className="inicio__boas-vindas" aria-labelledby="ola">
          <h1 id="ola">Olá, {usuario.nome.split(' ')[0]}</h1>
          {estatisticas ? (
            <Estatisticas e={estatisticas} />
          ) : erroEstatisticas ? (
            <p className="erro">
              Não deu para carregar o seu tempo de estudo.{' '}
              <button type="button" className="botao-link" onClick={() => carregarEstatisticas()}>
                Tentar de novo
              </button>
            </p>
          ) : (
            <div className="esqueleto" style={{ height: 150 }} />
          )}
        </section>

        <section className="inicio__acoes" aria-label="Entrar ou criar uma sala">
          <EntrarPorCodigo />
          <CriarSala />
        </section>

        <section className="inicio__salas" aria-labelledby="salas-abertas">
          <div className="titulo-secao">
            <h2 id="salas-abertas">Salas abertas</h2>
            <button type="button" className="botao-link" onClick={carregar}>
              Atualizar
            </button>
          </div>
          {erro && <p className="erro">{erro}</p>}
          {!salas && !erro && <div className="esqueleto" style={{ height: 180 }} />}
          {salas && <ListaSalas salas={salas.publicas} vazio="Nenhuma sala pública ainda. Crie a primeira!" />}

          {salas?.minhas.length > 0 && (
            <>
              <h2 className="titulo-secao">Suas salas</h2>
              <ListaSalas salas={salas.minhas} mostrarPrivada />
            </>
          )}
        </section>

        <aside className="inicio__ranking" aria-labelledby="ranking-titulo">
          <Ranking titulo="Quem mais focou" idTitulo="ranking-titulo" buscar={api.ranking} destaque={usuario.id} />
        </aside>
      </main>
    </>
  )
}

function Estatisticas({ e }) {
  const maior = Math.max(...e.ultimosDias.map((d) => d.segundos), 1)
  return (
    <div className="estatisticas">
      <dl className="estatisticas__numeros">
        <div>
          <dt>Hoje</dt>
          <dd>{duracao(e.hojeSegundos)}</dd>
        </div>
        <div>
          <dt>Nesta semana</dt>
          <dd>{duracao(e.semanaSegundos)}</dd>
        </div>
        <div>
          <dt>Sequência</dt>
          <dd>
            {e.sequencia} {e.sequencia === 1 ? 'dia' : 'dias'}
          </dd>
        </div>
      </dl>
      <figure className="barras" aria-label="Minutos de foco nos últimos 7 dias">
        {e.ultimosDias.map((d, i) => (
          <div key={d.data} className="barras__dia" title={`${diaCurto(d.data)}: ${duracao(d.segundos)}`}>
            <div className="barras__trilho">
              <div
                className={`barras__barra ${i === 6 ? 'barras__barra--hoje' : ''}`}
                style={{ height: `${Math.max(d.segundos ? 6 : 0, (d.segundos / maior) * 100)}%` }}
              />
            </div>
            <span>{diaCurto(d.data)}</span>
          </div>
        ))}
      </figure>
    </div>
  )
}

function ListaSalas({ salas, vazio, mostrarPrivada = false }) {
  if (!salas.length) return <p className="vazio">{vazio}</p>
  return (
    <ul className="lista-salas">
      {salas.map((s) => (
        <li key={s.codigo}>
          <button type="button" className="sala-linha" onClick={() => navegar(`/sala/${s.codigo}`)}>
            <span className={`fase-ponto fase-ponto--${s.online ? s.fase || 'foco' : 'vazia'}`} aria-hidden="true" />
            <span className="sala-linha__textos">
              <strong>{s.nome}</strong>
              <span>
                {s.descricao || `Criada por ${s.donoNome}`}
                {mostrarPrivada && !s.publica && ' (privada)'}
              </span>
            </span>
            <span className="sala-linha__estado">
              {s.online ? (
                <>
                  <strong>
                    {s.online} {s.online === 1 ? 'pessoa' : 'pessoas'}
                  </strong>
                  <span>{s.rodando ? `em ${NOME_FASE[s.fase].toLowerCase()}` : 'relógio parado'}</span>
                </>
              ) : (
                <span>
                  {s.focoMin} min de foco, {s.pausaMin} de pausa
                </span>
              )}
            </span>
          </button>
        </li>
      ))}
    </ul>
  )
}

function EntrarPorCodigo() {
  const [codigo, setCodigo] = useState('')
  const [erro, setErro] = useState(null)
  async function enviar(e) {
    e.preventDefault()
    setErro(null)
    try {
      const sala = await api.sala(codigo.trim())
      navegar(`/sala/${sala.codigo}`)
    } catch (err) {
      setErro(err.message)
    }
  }
  return (
    <form className="painel formulario" onSubmit={enviar}>
      <h2>Tenho um código</h2>
      <label className="campo">
        <span>Código da sala</span>
        <input
          value={codigo}
          onChange={(e) => setCodigo(e.target.value.toUpperCase().replace(/[^A-Z0-9]/g, ''))}
          maxLength={6}
          placeholder="Ex.: K7M2QX"
          className="campo__codigo"
          required
          minLength={6}
          autoCapitalize="characters"
        />
      </label>
      {erro && <p className="erro">{erro}</p>}
      <button className="botao">Entrar na sala</button>
    </form>
  )
}

function CriarSala() {
  const [nome, setNome] = useState('')
  const [descricao, setDescricao] = useState('')
  const [publica, setPublica] = useState(true)
  const [erro, setErro] = useState(null)
  const [enviando, setEnviando] = useState(false)
  async function enviar(e) {
    e.preventDefault()
    setErro(null)
    setEnviando(true)
    try {
      const sala = await api.criarSala(nome, descricao, publica)
      navegar(`/sala/${sala.codigo}`)
    } catch (err) {
      setErro(err.message)
      setEnviando(false)
    }
  }
  return (
    <form className="painel formulario" onSubmit={enviar}>
      <h2>Criar uma sala</h2>
      <label className="campo">
        <span>Nome</span>
        <input value={nome} onChange={(e) => setNome(e.target.value)} placeholder="Ex.: Revisão ENEM – Matemática" required minLength={3} maxLength={60} />
      </label>
      <label className="campo">
        <span>Descrição (opcional)</span>
        <input value={descricao} onChange={(e) => setDescricao(e.target.value)} maxLength={200} placeholder="O que vocês vão estudar" />
      </label>
      <label className="caixa">
        <input type="checkbox" checked={publica} onChange={(e) => setPublica(e.target.checked)} />
        <span>Aparece na lista de salas abertas</span>
      </label>
      {erro && <p className="erro">{erro}</p>}
      <button className="botao botao--principal" disabled={enviando}>
        {enviando ? 'Criando…' : 'Criar sala'}
      </button>
    </form>
  )
}
