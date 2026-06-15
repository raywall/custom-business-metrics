# Agent importavel

O agent desacopla a aplicacao do envio HTTP. Eventos entram em um buffer, sao agrupados e enviados
assincronamente para `POST /v1/metrics`.

Com o service executando em `localhost:8080`:

```bash
go run .
```

Para outro endpoint:

```bash
METRICS_SERVICE_ENDPOINT=https://metrics.example/v1/metrics go run .
```

Em aplicacoes de longa duracao, mantenha `agent.Run(ctx)` ativo durante todo o ciclo de vida e chame
`Close()` no shutdown. O fechamento drena os eventos aceitos no buffer antes de solicitar o envio
do lote final.
