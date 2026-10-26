import { useCallback, useEffect, useRef, useState } from 'react'
import Cabecalho from '../componentes/Cabecalho.jsx'
import ChamadaVideo from '../componentes/ChamadaVideo.jsx'
import Chat from '../componentes/Chat.jsx'
import ConfigSala from '../componentes/ConfigSala.jsx'
import Icone from '../componentes/Icone.jsx'
import Pessoas from '../componentes/Pessoas.jsx'
import Ranking from '../componentes/Ranking.jsx'
import Tarefas from '../componentes/Tarefas.jsx'
import Timer, { restanteAgora } from '../componentes/Timer.jsx'
import { api } from '../servicos/api.js'
import { NOME_FASE, relogio } from '../servicos/formato.js'
import { Link } from '../servicos/rota.jsx'
import { tocarAviso } from '../servicos/som.js'
import { useSala } from '../servicos/useSala.js'

export default function Sala({ codigo }) {
  const [existe, setExiste] = useState(null) // null = conferindo
  const [erro, setErro] = useState(null)

  useEffect(() => {
    api
      .sala(codigo)
      .then(() => setExiste(true))
      .catch((e) => {
        setExiste(false)
        setErro(e.message)
      })
  }, [codigo])

  if (existe === false) {
    return (
      <>
        <Cabecalho />
        <main className="sala-erro">
          <h1>Não deu para abrir a sala {codigo}</h1>
          <p>{erro}</p>
          <Link para="/" className="botao botao--principal">
            Voltar ao início
          </Link>
        </main>
      </>
    )
  }
  if (!existe) return <div className="carregando-tela" aria-busy="true" />
  return <SalaAberta codigo={codigo} />
}

const ABAS = [
  ['pessoas', 'Pessoas'],
  ['chat', 'Chat'],
  ['tarefas', 'Tarefas'],
  ['ranking', 'Ranking'],
]

function SalaAberta({ codigo }) {
  const chamadaRef = useRef(null)
  const aoSinal = useCallback((de, dados) => chamadaRef.current?.aoSinal(de, dados), [])
  const s = useSala(codigo, { aoSinal })
  const [aba, setAba] = useState('pessoas')
  const [configAberta, setConfigAberta] = useState(false)
  const [naChamada, setNaChamada] = useState(false)
  const [copiado, setCopiado] = useState(false)
  const [chatNovo, setChatNovo] = useState(0)
  const ultimaVista = useRef(0)

  // Som e notificação quando uma fase acaba.
  useEffect(() => {
    if (!s.ultimaFase) return
    const { comecou } = s.ultimaFase
    tocarAviso(comecou)
    if (document.hidden && 'Notification' in window && Notification.permission === 'granted') {
      new Notification(comecou === 'foco' ? 'Hora de focar' : 'Pausa!', {
        body: comecou === 'foco' ? 'A pausa acabou. Volte para a sala.' : `${NOME_FASE[comecou]}: levante, beba água, descanse a vista.`,
        tag: 'estuda-junto-fase',
      })
    }
  }, [s.ultimaFase])

  // O título da aba mostra o tempo, para acompanhar de outra aba.
  useEffect(() => {
    const atualizar = () => {
      if (!s.timer) return
      const r = restanteAgora(s.timer, s.diferencaRelogio)
      document.title = `${relogio(r)} ${NOME_FASE[s.timer.fase]} | ${s.sala?.nome || 'Estuda Junto'}`
    }
    atualizar()
    const id = setInterval(atualizar, 1000)
    return () => {
      clearInterval(id)
      document.title = 'Estuda Junto'
    }
  }, [s.timer, s.diferencaRelogio, s.sala?.nome])

  // Bolinha de mensagem nova quando o chat não está aberto.
  useEffect(() => {
    if (aba === 'chat') {
      ultimaVista.current = s.mensagens.length
      setChatNovo(0)
    } else {
      setChatNovo(Math.max(0, s.mensagens.length - ultimaVista.current))
    }
  }, [s.mensagens.length, aba])

  // Avisos somem sozinhos.
  useEffect(() => {
    if (!s.aviso) return
    const id = setTimeout(s.limparAviso, 5000)
    return () => clearTimeout(id)
  }, [s.aviso, s.limparAviso])

  async function copiarConvite() {
    const link = `${location.origin}/sala/${codigo}`
    try {
      await navigator.clipboard.writeText(link)
      setCopiado(true)
      setTimeout(() => setCopiado(false), 2000)
    } catch {
      window.prompt('Copie o link da sala:', link)
    }
  }

  function pedirNotificacao() {
    if ('Notification' in window && Notification.permission === 'default') Notification.requestPermission()
  }

  const emFoco = s.timer?.rodando && s.timer?.fase === 'foco'
  const buscarRanking = useCallback((periodo) => api.rankingSala(codigo, periodo), [codigo])

  return (
    <>
      <Cabecalho>
        {s.sala && (
          <div className="sala-topo">
            <Link para="/" className="sala-topo__voltar" aria-label="Voltar para as salas">
              <Icone nome="voltar" />
            </Link>
            <h1 className="sala-topo__nome">{s.sala.nome}</h1>
            <button type="button" className="codigo" onClick={copiarConvite} title="Copiar o link de convite">
              <span>{codigo}</span>
              <Icone nome="copiar" tamanho={16} />
              <span className="sr">Copiar link de convite</span>
            </button>
            {copiado && (
              <span className="sala-topo__copiado" role="status">
                Link copiado
              </span>
            )}
          </div>
        )}
      </Cabecalho>

      {s.conexao === 'reconectando' && (
        <p className="faixa-conexao" role="status">
          Reconectando à sala… (se o servidor estava dormindo, leva até 1 minuto)
        </p>
      )}

      <main className={`sala ${naChamada ? 'sala--chamada' : ''}`} onClick={pedirNotificacao}>
        <div className="sala__principal">
          {s.voce && (
            <ChamadaVideo
              ref={chamadaRef}
              voce={s.voce}
              participantes={s.chamada}
              limite={s.limiteChamada}
              enviar={s.enviar}
              aoMudarEstado={setNaChamada}
            />
          )}
          <Timer timer={s.timer} diferenca={s.diferencaRelogio} podeControlar={s.voce?.podeControlar} enviar={s.enviar} compacto={naChamada} />
          {s.voce?.dono && s.timer && (
            <button type="button" className="botao-link sala__config" onClick={() => setConfigAberta(true)}>
              <Icone nome="config" tamanho={16} /> Ajustar tempos
            </button>
          )}
        </div>

        <aside className="sala__lateral">
          <div className="abas" role="tablist" aria-label="Painéis da sala">
            {ABAS.map(([id, nome]) => (
              <button
                key={id}
                type="button"
                role="tab"
                id={`aba-${id}`}
                aria-selected={aba === id}
                aria-controls={`painel-${id}`}
                onClick={() => setAba(id)}
              >
                {nome}
                {id === 'pessoas' && <span className="abas__contador">{s.membros.length}</span>}
                {id === 'tarefas' && s.tarefas.some((t) => !t.feita) && <span className="abas__contador">{s.tarefas.filter((t) => !t.feita).length}</span>}
                {id === 'chat' && chatNovo > 0 && <span className="abas__novo" aria-label={`${chatNovo} novas`} />}
              </button>
            ))}
          </div>
          <div className="abas__painel" role="tabpanel" id={`painel-${aba}`} aria-labelledby={`aba-${aba}`}>
            {aba === 'pessoas' && <Pessoas membros={s.membros} voce={s.voce} donoId={s.sala?.donoId} enviar={s.enviar} />}
            {aba === 'chat' && <Chat mensagens={s.mensagens} voce={s.voce} enviar={s.enviar} emFoco={emFoco} />}
            {aba === 'tarefas' && <Tarefas tarefas={s.tarefas} enviar={s.enviar} />}
            {aba === 'ranking' && <Ranking titulo="Ranking da sala" buscar={buscarRanking} destaque={s.voce?.usuarioId} atualizarCada={60000} />}
          </div>
        </aside>
      </main>

      {s.aviso && (
        <div className="aviso" role="alert" key={s.aviso.em}>
          {s.aviso.texto}
        </div>
      )}

      {s.voce?.dono && s.timer && s.sala && (
        <ConfigSala aberta={configAberta} aoFechar={() => setConfigAberta(false)} timer={s.timer} sala={s.sala} enviar={s.enviar} />
      )}
    </>
  )
}
