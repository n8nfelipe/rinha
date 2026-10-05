# Carga com Gatling

Este teste cria pessoas exclusivamente através de `POST /pessoas`. Ele não acessa o PostgreSQL diretamente.

Requisitos: Java 17+ e Maven 3.9+.

Com a API em execução na raiz do projeto:

```bash
cd gatling
mvn -U gatling:test \
  -DbaseUrl=http://localhost:8080 \
  -Dtotal=50000 \
  -Dusers=25
```

O Gatling gera o relatório HTML em `target/gatling/`. Para uma execução rápida:

```bash
mvn -U gatling:test -Dtotal=100 -Dusers=4
```

Cada requisição envia `stack: ["Go", "PostgreSQL"]`, então, após a execução, a busca deve retornar pessoas:

Para dividir a carga entre usuários, use um valor de `total` divisível por `users`.

```bash
curl 'http://localhost:8080/pessoas?t=go'
```
