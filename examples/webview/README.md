# Stack completa com webview

Este exemplo provisiona as tres pecas e um gerador de metricas:

- `metrics-service`: API HTTP em `http://localhost:8080`;
- `metrics-agent`: receptor UDP em `localhost:8125`;
- `metrics-webview`: dashboard em `http://localhost:5173`;
- `metrics-generator`: eventos sinteticos para preencher a visualizacao.

Execute a partir deste diretorio:

```bash
docker compose up --build
```

Abra `http://localhost:5173`. Na configuracao do webview, mantenha a URL da API como
`http://localhost:8080`.

Para encerrar:

```bash
docker compose down
```
