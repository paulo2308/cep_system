# Sistema CEP - Distributed Tracing with OpenTelemetry

Sistema de consulta de CEP com rastreamento distribuído usando OpenTelemetry, Zipkin e Go.

### Componentes

- **Service-A**: API Gateway que recebe requisições de CEP
- **Service-B**: Serviço que consulta ViaCEP e WeatherAPI
- **OTEL Collector**: Coleta e processa traces
- **Zipkin**: Interface para visualização de traces

## 🚀 Como Executar

### Pré-requisitos

- Docker
- Docker Compose
- Make (opcional)

### 1. Configuração Inicial

```bash
# Clone o repositório
git clone <repository-url>
cd sistema_cep
```

### 2. Configure sua API Key

Edite o arquivo `.env` e adicione sua chave da WeatherAPI:

```bash
# Obtenha uma chave gratuita em: https://www.weatherapi.com/
WEATHER_API_KEY=sua_chave_aqui
```

### 3. Execute o Sistema

```bash
# usando Docker Compose
docker-compose up -d
```

## 📡 Endpoints

| Serviço | URL | Descrição |
|---------|-----|-----------|
| **Service-A** | `POST http://localhost:8081/cep` | API principal |
| **Service-B** | `GET http://localhost:8080/weather` | API de clima |
| **Zipkin UI** | `http://localhost:9411` | Interface de tracing |

## 🧪 Testando o Sistema

### Teste com CEP Válido

```bash
curl -X POST http://localhost:8081/cep \
  -H "Content-Type: application/json" \
  -d '{"cep":"01310100"}'
```

**Resposta esperada:**
```json
{
  "city": "São Paulo",
  "temp_C": 22.5,
  "temp_F": 72.5,
  "temp_K": 295.5
}
```

### Teste com CEP Inválido

```bash
curl -X POST http://localhost:8081/cep \
  -H "Content-Type: application/json" \
  -d '{"cep":"00000000"}'
```

**Resposta esperada:**
```
can not find zipcode (HTTP 404)
```

## 🔧 Desenvolvimento

### Executar em Modo Desenvolvimento

```bash
# Rebuild e start
docker-compose up --build
```

### Logs em Tempo Real

```bash
# Serviço específico
docker-compose logs -f service-a
```

## 🔒 Segurança

## 📝 Variáveis de Ambiente

| Variável | Descrição | Padrão |
|----------|-----------|--------|
| `WEATHER_API_KEY` | Chave da WeatherAPI | *obrigatório* |
| `SERVICE_A_PORT` | Porta do Service-A | `8081` |
| `SERVICE_B_PORT` | Porta do Service-B | `8080` |
| `ZIPKIN_PORT` | Porta do Zipkin | `9411` |
| `OTEL_HTTP_PORT` | Porta OTLP HTTP | `4318` |
| `OTEL_GRPC_PORT` | Porta OTLP gRPC | `4317` |
