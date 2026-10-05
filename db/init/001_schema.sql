CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS people (
    email text PRIMARY KEY,
    name text NOT NULL,
    address text NOT NULL,
    phone text NOT NULL,
    birth_date date NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS people_name_idx ON people (name);
CREATE INDEX IF NOT EXISTS people_updated_at_idx ON people (updated_at DESC);

CREATE TABLE IF NOT EXISTS pessoas (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    apelido varchar(32) NOT NULL UNIQUE,
    nome varchar(100) NOT NULL,
    nascimento date NOT NULL,
    stack text[] NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS pessoas_nome_idx ON pessoas (nome);
CREATE INDEX IF NOT EXISTS pessoas_apelido_idx ON pessoas (apelido);
