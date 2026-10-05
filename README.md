# Rinha API

API em Go com PostgreSQL para cadastro, consulta e carga de pessoas. O fluxo principal usa a tabela `pessoas` e atende os três endpoints do contrato:

- `POST /pessoas`
- `GET /pessoas/{id}`
- `GET /pessoas?t=termo`

A carga de performance usa Gatling e também chama exclusivamente `POST /pessoas`.

## Requisitos

Para executar a API:

- Docker com Docker Compose;
- portas `8080` e `5432` disponíveis;
- porta `8090` disponível quando a interface de carga for usada.

Para executar Gatling localmente, além da API:

- Java 17 ou superior;
- Maven 3.9 ou superior.

O serviço Gatling no Compose já inclui Java e Maven, portanto Java/Maven locais não são necessários quando a carga for executada via Docker.

## Execução rápida

Suba o PostgreSQL e a API:

```bash
docker compose up --build -d
```

Verifique a saúde da API:

```bash
curl http://localhost:8080/healthz
```

Resposta esperada:

```json
{"status":"ok"}
```

Abra o painel operacional em <http://localhost:8080/>.

O painel de carga do Gatling é iniciado separadamente pelo profile `load` e fica disponível em <http://localhost:8090/>.

## Endpoints principais

### Criar pessoa

`POST /pessoas` cria uma pessoa na tabela `pessoas`.

```bash
curl -i -X POST http://localhost:8080/pessoas \
  -H 'Content-Type: application/json' \
  -d '{
    "apelido": "ana",
    "nome": "Ana Silva",
    "nascimento": "1990-05-20",
    "stack": ["Go", "PostgreSQL"]
  }'
```

Sucesso: `201 Created`. O corpo contém o UUID e o header `Location` aponta para `GET /pessoas/{id}`.

Regras principais:

- `apelido`: obrigatório, até 32 caracteres e único;
- `nome`: obrigatório, até 100 caracteres;
- `nascimento`: formato `YYYY-MM-DD` e não pode ser futuro;
- `stack`: opcional, com no máximo 20 tecnologias de até 32 caracteres.

### Buscar por UUID

```bash
curl http://localhost:8080/pessoas/SEU_UUID
```

Respostas possíveis:

- `200 OK`: pessoa encontrada;
- `400 Bad Request`: UUID inválido;
- `404 Not Found`: UUID inexistente.

### Pesquisar por termo

`GET /pessoas?t=termo` pesquisa parcialmente, sem diferenciar maiúsculas e minúsculas, nos campos `nome`, `apelido` e em cada item de `stack`.

```bash
curl 'http://localhost:8080/pessoas?t=go'
curl 'http://localhost:8080/pessoas?t=postgres'
curl 'http://localhost:8080/pessoas?t=ana'
```

O parâmetro `t` é obrigatório. Sem ele, a API retorna `400 Bad Request`.

## Carga com Gatling

A simulação em [`gatling/src/test/java/rinha/CreatePeopleSimulation.java`](gatling/src/test/java/rinha/CreatePeopleSimulation.java) não acessa o banco diretamente. Cada operação é um `POST /pessoas` com uma pessoa contendo:

```json
{
  "apelido": "...",
  "nome": "Pessoa Go ...",
  "nascimento": "1990-01-15",
  "stack": ["Go", "PostgreSQL"]
}
```

### Interface web pelo Docker Compose

O serviço está no profile `load` para não iniciar a interface durante um `docker compose up` normal:

```bash
docker compose --profile load up --build gatling
```

Abra <http://localhost:8090/> para iniciar uma simulação, acompanhar os logs e abrir os relatórios HTML.

O formulário começa preenchido com uma carga pequena para validação, mas nenhuma carga é executada automaticamente. A configuração de referência é:

- 50.000 inserts;
- 25 usuários concorrentes;
- 2.000 requisições por usuário.

### Executar localmente

```bash
cd gatling
mvn -U gatling:test \
  -DbaseUrl=http://localhost:8080 \
  -Dtotal=50000 \
  -Dusers=25
```

Para um teste rápido:

```bash
cd gatling
mvn -U gatling:test -Dtotal=100 -Dusers=4
```

O relatório HTML fica em `gatling/target/gatling/`. A simulação exige que `total` seja divisível por `users` e valida que todas as requisições retornem `201`.

## Painel e métricas

O painel em <http://localhost:8080/> atualiza automaticamente e exibe:

- total de pessoas inseridas;
- total de erros;
- tempo total acumulado;
- duração da última operação;
- histórico recente.

Os mesmos dados estão disponíveis em:

```bash
curl http://localhost:8080/api/metrics
```

Campos principais:

| Campo | Descrição |
| --- | --- |
| `total_inserted` | Inserts concluídos com sucesso |
| `total_errors` | Operações que falharam |
| `total_duration_ms` | Tempo acumulado das operações observadas |
| `last_duration_ms` | Duração da última operação |
| `recent_batches` | Histórico recente com status, quantidade e duração |

## API legada de registros

O projeto ainda mantém endpoints separados para a tabela `people`. Eles não alimentam a tabela `pessoas` e não participam da carga Gatling:

### Inserção em lote

`POST /v1/records/bulk` aceita de 1 até `MAX_BATCH_SIZE` registros e retorna `202 Accepted`.

```bash
curl -X POST http://localhost:8080/v1/records/bulk \
  -H 'Content-Type: application/json' \
  -d '{
    "records": [{
      "name": "Ana Silva",
      "address": "Rua A, 100",
      "phone": "+5511999999999",
      "birth_date": "1990-05-20",
      "email": "ana@example.com"
    }]
  }'
```

### Buscar registro por email

```bash
curl http://localhost:8080/v1/records/ana@example.com
```

### Listar registros

```bash
curl 'http://localhost:8080/v1/records?prefix=Ana&limit=100&offset=0'
```

`limit` varia de 1 a 1.000 e `offset` não pode ser negativo.

## Configuração da API

Variáveis de ambiente:

| Variável | Padrão | Uso |
| --- | --- | --- |
| `DATABASE_URL` | PostgreSQL local | URL de conexão com o banco |
| `HTTP_ADDR` | `:8080` | Endereço HTTP |
| `DB_MAX_CONNS` | `24` | Máximo de conexões no pool |
| `DB_MIN_CONNS` | `4` | Mínimo de conexões no pool |
| `MAX_BATCH_SIZE` | `50000` | Limite do endpoint legado em lote |

No Compose, essas variáveis já são configuradas para os serviços `api` e `db`.

## Estrutura do projeto

```text
cmd/api/                         aplicação HTTP
internal/config/                 configuração por ambiente
internal/httpapi/                rotas, validações, métricas e painel
internal/store/                  acesso ao PostgreSQL
db/init/                         criação inicial das tabelas
gatling/                         simulação de carga Gatling
gatling/web/                     interface web e acesso aos relatórios
compose.yaml                     API, PostgreSQL e profile de carga
Dockerfile                       build da API Go
```

## Solução de problemas

### A busca `t=go` retorna `[]`

Confirme que existem pessoas em `pessoas`:

```bash
curl -X POST http://localhost:8080/pessoas \
  -H 'Content-Type: application/json' \
  -d '{"apelido":"gopher","nome":"Pessoa Go","nascimento":"1990-05-20","stack":["Go"]}'
```

Depois consulte novamente:

```bash
curl 'http://localhost:8080/pessoas?t=go'
```

Registros enviados para `/v1/records/bulk` ficam na tabela `people` e não aparecem nessa busca.

### O Gatling parece parado

Confirme se o serviço da interface está em execução:

```bash
docker compose --profile load ps
```

Depois abra <http://localhost:8090/>, mantenha uma carga divisível — por exemplo, `100` requisições e `4` usuários — e clique em **Iniciar carga**. Na primeira execução, o Maven baixa as dependências do Gatling. O relatório aparece no painel ao final da simulação.

### Recriar o banco

Os scripts em `db/init` só são executados quando o volume é criado pela primeira vez. Para recriar o banco, removendo os dados persistidos:

```bash
docker compose down -v
docker compose up --build -d
```

Use esse comando somente quando puder descartar os dados locais.

## Encerrar

```bash
docker compose down
```

Para também remover o volume local do PostgreSQL:

```bash
docker compose down -v
```
