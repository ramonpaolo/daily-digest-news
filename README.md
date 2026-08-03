# Daily Digest News

Um pequeno job em Go que coleta as principais histórias do Hacker News, usa a IA compatível com OpenAI da Zenifra para criar um resumo em português do Brasil e envia o resultado por SMTP.

O projeto é público para estudo. A aplicação publicada na Zenifra deve permanecer privada e não expõe um endpoint de disparo de e-mail.

## Como funciona

Ao iniciar ou reiniciar, o processo executa um digest imediatamente. Depois, continua executando todos os dias às 08:00 no fuso `America/Sao_Paulo`:

1. consulta a API oficial do Hacker News;
2. seleciona dez histórias e tenta extrair o texto dos artigos;
3. envia o lote delimitado como conteúdo não confiável para a Zenifra AI;
4. valida a resposta JSON da LLM;
5. monta um e-mail HTML e texto e envia por SMTP.

As chamadas da Zenifra AI têm timeout de resposta de 5 minutos por tentativa; a coleta HTTP geral mantém timeout menor e separado.

## Observabilidade

Os logs são texto estruturado e começam com `component` e `event`. Cada execução registra início, duração, tentativas e resultado de coleta do Hacker News, extração de artigos, chamada da LLM, renderização e cada etapa SMTP; a resposta da LLM inclui apenas metadados como status HTTP, quantidade de escolhas, bytes de conteúdo/raciocínio e `finish_reason`. Prompts, artigos, corpos de e-mail, tokens, senhas e partes locais dos endereços nunca são registrados.

Em Kubernetes, use `kubectl logs` no pod do projeto e filtre por componente ou evento, por exemplo `component=llm`, `event=response` ou `event=stage_failed`. Uma resposta da LLM sem conteúdo agora aparece com `choices`, `content_bytes`, `reasoning_bytes`, `refusal_bytes` e `tool_calls`, permitindo distinguir resposta vazia, raciocínio sem resposta final e falha de transporte.

O estado é mantido somente em memória. Um reinício dispara deliberadamente uma nova execução e pode enviar outro e-mail no mesmo dia; enquanto o mesmo processo permanece ativo, o agendamento diário evita duplicatas.

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
| `TIMEZONE` | não | Default `America/Sao_Paulo` |
| `TOP_STORIES` | não | Default `10`, máximo `20` |

## Health check

`GET /health` retorna apenas `{"status":"ok"}`. A aplicação escuta em `0.0.0.0:8080` para que a plataforma possa validar o processo; o projeto Zenifra usa exposição privada.

## Deploy OCI

O runtime GitHub nativo da Zenifra não oferece Go neste momento, então o workflow cria uma imagem OCI no GHCR. Cada imagem usa uma tag imutável baseada no SHA do commit; `latest` não é utilizado.

Depois do primeiro build, torne o pacote GHCR público, crie o projeto HTTP privado na Zenifra e configure as ENVs diretamente no console. Para os deploys seguintes, configure:

- `ZENIFRA_DEPLOY_API_KEY` como GitHub Actions Secret;
- `ZENIFRA_PROJECT_ID` como GitHub Actions Variable.

O workflow só promove uma imagem após os testes e o scan passarem.