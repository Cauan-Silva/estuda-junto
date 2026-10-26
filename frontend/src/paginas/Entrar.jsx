import { useState } from 'react'
import { api } from '../servicos/api.js'
import { useSessao } from '../servicos/sessao.jsx'
import Marca from '../componentes/Marca.jsx'

/** Login e cadastro na mesma tela. Quem abriu um link de sala volta para ela depois de entrar. */
export default function Entrar() {
  const { entrar } = useSessao()
  const [modo, setModo] = useState('entrar') // entrar | cadastrar
  const [nome, setNome] = useState('')
  const [email, setEmail] = useState('')
  const [senha, setSenha] = useState('')
  const [erro, setErro] = useState(null)
  const [enviando, setEnviando] = useState(false)
  const cadastro = modo === 'cadastrar'
  const convite = location.pathname.match(/^\/sala\/([A-Za-z0-9]{6})/)

  async function enviar(e) {
    e.preventDefault()
    setErro(null)
    setEnviando(true)
    try {
      entrar(cadastro ? await api.cadastrar(nome, email, senha) : await api.entrar(email, senha))
    } catch (err) {
      setErro(err.message)
      setEnviando(false)
    }
  }

  return (
    <main className="entrar">
      <section className="entrar__apresentacao">
        <Marca grande />
        <h1 className="entrar__titulo">Estudar sozinho cansa. Aqui você estuda junto.</h1>
        <ul className="entrar__lista">
          <li>
            <span className="fase-ponto fase-ponto--foco" aria-hidden="true" />
            Um relógio Pomodoro só, igual para todo mundo da sala
          </li>
          <li>
            <span className="fase-ponto fase-ponto--pausa" aria-hidden="true" />
            Chat e chamada de vídeo nas pausas, com tela compartilhada
          </li>
          <li>
            <span className="fase-ponto fase-ponto--pausa_longa" aria-hidden="true" />
            Quadro de tarefas da turma e ranking de horas da semana
          </li>
        </ul>
      </section>

      <section className="entrar__cartao" aria-labelledby="entrar-titulo">
        {convite && <p className="entrar__convite">Você foi convidado para a sala {convite[1].toUpperCase()}. Entre para participar.</p>}
        <h2 id="entrar-titulo">{cadastro ? 'Criar conta' : 'Entrar'}</h2>
        <form onSubmit={enviar} className="formulario">
          {cadastro && (
            <label className="campo">
              <span>Nome</span>
              <input value={nome} onChange={(e) => setNome(e.target.value)} autoComplete="name" required minLength={2} maxLength={60} />
            </label>
          )}
          <label className="campo">
            <span>E-mail</span>
            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" required />
          </label>
          <label className="campo">
            <span>Senha</span>
            <input
              type="password"
              value={senha}
              onChange={(e) => setSenha(e.target.value)}
              autoComplete={cadastro ? 'new-password' : 'current-password'}
              required
              minLength={cadastro ? 8 : undefined}
            />
            {cadastro && <small>Pelo menos 8 caracteres.</small>}
          </label>
          {erro && (
            <p className="erro" role="alert">
              {erro}
            </p>
          )}
          <button className="botao botao--principal" disabled={enviando}>
            {enviando ? 'Aguarde…' : cadastro ? 'Criar conta' : 'Entrar'}
          </button>
        </form>
        <p className="entrar__trocar">
          {cadastro ? 'Já tem conta?' : 'Primeira vez aqui?'}{' '}
          <button
            type="button"
            className="botao-link"
            onClick={() => {
              setModo(cadastro ? 'entrar' : 'cadastrar')
              setErro(null)
            }}
          >
            {cadastro ? 'Entrar' : 'Criar uma conta'}
          </button>
        </p>
      </section>
    </main>
  )
}
