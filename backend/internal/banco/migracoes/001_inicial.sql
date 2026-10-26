CREATE TABLE usuarios (
    id          BIGSERIAL PRIMARY KEY,
    nome        VARCHAR(60)  NOT NULL,
    email       VARCHAR(160) NOT NULL,
    senha_hash  VARCHAR(200) NOT NULL,
    criado_em   TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX usuarios_email ON usuarios (lower(email));

CREATE TABLE salas (
    id                 BIGSERIAL PRIMARY KEY,
    codigo             CHAR(6)      NOT NULL UNIQUE,
    nome               VARCHAR(60)  NOT NULL,
    descricao          VARCHAR(200) NOT NULL DEFAULT '',
    publica            BOOLEAN      NOT NULL DEFAULT TRUE,
    dono_id            BIGINT       NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    foco_min           INT          NOT NULL DEFAULT 25,
    pausa_min          INT          NOT NULL DEFAULT 5,
    pausa_longa_min    INT          NOT NULL DEFAULT 15,
    ciclos_ate_longa   INT          NOT NULL DEFAULT 4,
    todos_controlam    BOOLEAN      NOT NULL DEFAULT FALSE,
    criada_em          TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX salas_publicas ON salas (publica, criada_em DESC);

CREATE TABLE tarefas (
    id         BIGSERIAL PRIMARY KEY,
    sala_id    BIGINT       NOT NULL REFERENCES salas (id) ON DELETE CASCADE,
    autor_id   BIGINT       NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    texto      VARCHAR(200) NOT NULL,
    feita      BOOLEAN      NOT NULL DEFAULT FALSE,
    feita_por  BIGINT       REFERENCES usuarios (id) ON DELETE SET NULL,
    criada_em  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX tarefas_sala ON tarefas (sala_id, id);

CREATE TABLE mensagens (
    id         BIGSERIAL PRIMARY KEY,
    sala_id    BIGINT       NOT NULL REFERENCES salas (id) ON DELETE CASCADE,
    autor_id   BIGINT       NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    texto      VARCHAR(500) NOT NULL,
    criada_em  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX mensagens_sala ON mensagens (sala_id, id DESC);

-- Cada trecho de foco que a pessoa fez numa sala (base do ranking e das estatísticas).
CREATE TABLE sessoes_estudo (
    id          BIGSERIAL PRIMARY KEY,
    usuario_id  BIGINT      NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    sala_id     BIGINT      REFERENCES salas (id) ON DELETE SET NULL,
    inicio      TIMESTAMPTZ NOT NULL,
    segundos    INT         NOT NULL CHECK (segundos > 0)
);
CREATE INDEX sessoes_usuario ON sessoes_estudo (usuario_id, inicio);
CREATE INDEX sessoes_inicio ON sessoes_estudo (inicio);
