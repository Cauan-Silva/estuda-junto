// Package tempo_real é o coração do projeto: cada sala aberta é uma goroutine (um "ator")
// que é a única dona do estado da sala (relógio, quem está online, tarefas, chat e chamada).
// Os navegadores conversam com ela por WebSocket; nada é compartilhado entre goroutines
// além de canais, então não há trava (mutex) no estado da sala.
package tempo_real

import (
	"encoding/json"
	"time"

	"github.com/Cauan-Silva/estuda-junto/internal/pomodoro"
	"github.com/Cauan-Silva/estuda-junto/internal/salas"
)

// Entrada é o que o navegador manda. Só os campos do "tipo" escolhido são usados.
//
//	chat           {texto}
//	timer          {acao: iniciar | pausar | pular | reiniciar}
//	config         {config}                    (só o dono)
//	status         {texto}                     o que a pessoa está estudando
//	tarefa_nova    {texto}
//	tarefa_marcar  {id, feita}
//	tarefa_apagar  {id}
//	chamada_entrar {camera, microfone}
//	chamada_sair   {}
//	midia          {camera, microfone, tela}   liga/desliga câmera, microfone ou tela
//	sinal          {para, dados}               WebRTC: oferta, resposta ou candidato
type Entrada struct {
	Tipo      string          `json:"tipo"`
	Texto     string          `json:"texto,omitempty"`
	Acao      string          `json:"acao,omitempty"`
	ID        int64           `json:"id,omitempty"`
	Feita     bool            `json:"feita,omitempty"`
	Config    *ConfigSala     `json:"config,omitempty"`
	Camera    bool            `json:"camera,omitempty"`
	Microfone bool            `json:"microfone,omitempty"`
	Tela      bool            `json:"tela,omitempty"`
	Para      string          `json:"para,omitempty"`
	Dados     json.RawMessage `json:"dados,omitempty"`
}

type ConfigSala struct {
	FocoMin        int  `json:"focoMin"`
	PausaMin       int  `json:"pausaMin"`
	PausaLongaMin  int  `json:"pausaLongaMin"`
	CiclosAteLonga int  `json:"ciclosAteLonga"`
	TodosControlam bool `json:"todosControlam"`
}

func (c ConfigSala) Pomodoro() pomodoro.Config {
	return pomodoro.Config{
		Foco:           time.Duration(c.FocoMin) * time.Minute,
		Pausa:          time.Duration(c.PausaMin) * time.Minute,
		PausaLonga:     time.Duration(c.PausaLongaMin) * time.Minute,
		CiclosAteLonga: c.CiclosAteLonga,
	}
}

// Membro é uma pessoa online na sala (várias abas contam como uma pessoa).
type Membro struct {
	UsuarioID int64  `json:"usuarioId"`
	Nome      string `json:"nome"`
	Status    string `json:"status"`
	Focando   bool   `json:"focando"`
	NaChamada bool   `json:"naChamada"`
	conexoes  int
}

// Participante é uma conexão (aba) dentro da chamada de vídeo.
type Participante struct {
	ID        string `json:"id"` // id da conexão, usado como destino dos sinais WebRTC
	UsuarioID int64  `json:"usuarioId"`
	Nome      string `json:"nome"`
	Camera    bool   `json:"camera"`
	Microfone bool   `json:"microfone"`
	Tela      bool   `json:"tela"`
}

// InfoSala é a parte da sala que o navegador mostra.
type InfoSala struct {
	Codigo         string `json:"codigo"`
	Nome           string `json:"nome"`
	Descricao      string `json:"descricao"`
	Publica        bool   `json:"publica"`
	DonoID         int64  `json:"donoId"`
	DonoNome       string `json:"donoNome"`
	TodosControlam bool   `json:"todosControlam"`
}

type Voce struct {
	ConexaoID     string `json:"conexaoId"`
	UsuarioID     int64  `json:"usuarioId"`
	Dono          bool   `json:"dono"`
	PodeControlar bool   `json:"podeControlar"`
}

// Saida é o que o servidor manda. Cada tipo preenche só os seus campos.
//
//	estado   tudo, logo ao entrar
//	timer    o relógio mudou
//	fase     uma fase acabou (para tocar o som)
//	membros  quem está online mudou
//	chat     mensagem nova
//	tarefas  a lista de tarefas mudou
//	sala     nome/configuração mudou (e o "voce", porque o poder de controlar pode mudar)
//	chamada  quem está na chamada mudou
//	sinal    WebRTC vindo de outra conexão
//	erro     algo foi recusado
type Saida struct {
	Tipo          string           `json:"tipo"`
	Sala          *InfoSala        `json:"sala,omitempty"`
	Voce          *Voce            `json:"voce,omitempty"`
	Timer         *pomodoro.Estado `json:"timer,omitempty"`
	Membros       []Membro         `json:"membros,omitempty"`
	Tarefas       []salas.Tarefa   `json:"tarefas,omitempty"`
	Mensagens     []salas.Mensagem `json:"mensagens,omitempty"`
	Mensagem      *salas.Mensagem  `json:"mensagem,omitempty"`
	Chamada       []Participante   `json:"chamada,omitempty"`
	Terminou      pomodoro.Fase    `json:"terminou,omitempty"`
	Comecou       pomodoro.Fase    `json:"comecou,omitempty"`
	De            string           `json:"de,omitempty"`
	Dados         json.RawMessage  `json:"dados,omitempty"`
	Erro          string           `json:"erro,omitempty"`
	LimiteChamada int              `json:"limiteChamada,omitempty"`
}
