# Daily Digest News

Um pequeno job em Go que coleta notícias do Hacker News e do IEEE Spectrum e também envia lições curtas de matemática, computação e física. A IA compatível com OpenAI da Zenifra gera os conteúdos em português do Brasil e o resultado é enviado por SMTP.

O projeto é público para estudo. A aplicação publicada na Zenifra deve permanecer privada e não expõe um endpoint de disparo de e-mail.

## Como funciona

Ao iniciar ou reiniciar, o processo executa um digest de notícias e uma lição de aprendizado imediatamente. Depois, continua executando no fuso `America/Sao_Paulo`:

1. consulta as fontes nativas (Hacker News e o RSS oficial do IEEE Spectrum) em paralelo;
2. intercala as fontes até o limite global de `TOP_STORIES`, remove URLs duplicadas e tenta extrair o texto dos artigos;
3. envia o lote delimitado como conteúdo não confiável para a Zenifra AI;
4. valida a resposta JSON da LLM;
5. monta um e-mail HTML e texto e envia por SMTP.

Além disso, uma lição autocontida de 10–15 minutos é enviada às `07:00` e `18:00`. O formato alterna de forma adaptativa entre texto explicativo e pergunta com resposta comentada. Os temas padrão incluem matemática, computação, Go, estruturas de dados e algoritmos, internals de sistemas operacionais, internals de bancos de dados e física.

As chamadas da Zenifra AI têm timeout de resposta de 5 minutos por tentativa; a coleta HTTP geral mantém timeout menor e separado.

## Observabilidade

Os logs são texto estruturado e começam com `component` e `event`. Cada execução registra início, duração, tentativas e resultado de coleta do Hacker News, extração de artigos, chamada da LLM, renderização e cada etapa SMTP; a resposta da LLM inclui apenas metadados como status HTTP, quantidade de escolhas, bytes de conteúdo/raciocínio e `finish_reason`. Prompts, artigos, corpos de e-mail, tokens, senhas e partes locais dos endereços nunca são registrados.

Em Kubernetes, use `kubectl logs` no pod do projeto e filtre por componente ou evento, por exemplo `component=llm`, `event=response` ou `event=stage_failed`. Uma resposta da LLM sem conteúdo agora aparece com `choices`, `content_bytes`, `reasoning_bytes`, `refusal_bytes` e `tool_calls`, permitindo distinguir resposta vazia, raciocínio sem resposta final e falha de transporte.

O histórico de aprendizado é persistido em SQLite. Configure um volume persistente para `/data`; o arquivo padrão é `/data/daily-digest-news.sqlite3` e os arquivos WAL/SHM ficam no mesmo diretório. Um reinício dispara deliberadamente uma nova lição `startup`, enquanto os slots `morning` e `evening` são deduplicados por data mesmo após reinícios.

## Desenvolvimento local

Requisitos: Go 1.26.5.

```bash
cp .env.example .env
# Edite .env somente com valores locais; nunca faça commit desse arquivo.
set -a && . ./.env && set +a
go test -race ./...
go run ./cmd/daily-digest-news run-once
go run ./cmd/daily-digest-news serve
```

O modo `run-once` envia um e-mail real. Para testar apenas a coleta e a LLM, use doubles nos testes; não coloque suas chaves em scripts ou fixtures.

## Variáveis de ambiente

| Variável | Obrigatória | Descrição |
| --- | --- | --- |
| `ZENIFRA_AI_API_KEY` | sim | Chave criada no console da Zenifra |
| `ZENIFRA_AI_MODEL` | sim | Modelo autorizado para a chave |
| `ZENIFRA_AI_BASE_URL` | não | Default `https://ai.zenifra.com/v1` |
| `SMTP_HOST`, `SMTP_PORT` | sim | Servidor e porta SMTP |
| `SMTP_SECURITY` | não | `starttls` (default) ou `tls` |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | sim | Credencial SMTP |
| `SMTP_FROM`, `EMAIL_TO` | sim | Remetente e destinatário |
| `PORT` | não | Default `8080` |
| `SCHEDULE_TIME` | não | Default `08:00` |
| `LEARNING_MORNING_TIME` | não | Default `07:00` |
| `LEARNING_EVENING_TIME` | não | Default `18:00` |
| `LEARNING_TOPICS` | não | Lista separada por vírgulas; substitui os temas padrão |
| `TIMEZONE` | não | Default `America/Sao_Paulo` |
| `TOP_STORIES` | não | Limite global do digest; default `10`, máximo `20` |
| `SQLITE_PATH` | não | Arquivo SQLite; default `/data/daily-digest-news.sqlite3` |
| `LEARNING_RETENTION_DAYS` | não | Retenção das lições; default `365`, `0` desativa a limpeza |

As fontes são embutidas no código e ficam ativas por padrão. Uma indisponibilidade isolada é registrada e não impede o envio com as demais fontes; o digest falha somente quando nenhuma fonte retorna notícias utilizáveis. Cada item do email identifica sua fonte. Para adicionar outra fonte, implemente `news.Provider`, adicione testes do adaptador e registre-o no entrypoint.

O fluxo de aprendizado é independente do digest de notícias: uma falha em uma lição não bloqueia as notícias. Os temas configurados são persistidos como interesses ativos e a próxima lição prioriza o menos utilizado. As últimas oito lições enviadas orientam a LLM a evitar repetição, sem reenviar os textos completos.

## Health check

`GET /health` retorna apenas `{"status":"ok"}`. A aplicação escuta em `0.0.0.0:8080` para que a plataforma possa validar o processo; o projeto Zenifra usa exposição privada.

## Deploy OCI

O runtime GitHub nativo da Zenifra não oferece Go neste momento, então o workflow cria uma imagem OCI no GHCR. Cada imagem usa uma tag imutável baseada no SHA do commit; `latest` não é utilizado.

Depois do primeiro build, torne o pacote GHCR público, crie o projeto HTTP privado na Zenifra e configure as ENVs diretamente no console. Para os deploys seguintes, configure:

Na Zenifra, monte um volume persistente gravável pelo usuário `nonroot` no caminho `/data`. Use uma única réplica ativa para este SQLite. O backup deve preservar o arquivo `.sqlite3` e os arquivos WAL/SHM; faça-o com a aplicação parada ou com uma ferramenta SQLite compatível.

- `ZENIFRA_DEPLOY_API_KEY` como GitHub Actions Secret;
- `ZENIFRA_PROJECT_ID` como GitHub Actions Variable.

O workflow só promove uma imagem após os testes e o scan passarem.
