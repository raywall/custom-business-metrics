# Service importavel

Este exemplo instancia a API HTTP do Custom Business Metrics com armazenamento em memoria.

```bash
go run .
curl http://localhost:8080/health
```

Para persistencia em DynamoDB:

```go
service, err := metrics.New(ctx, metrics.Config{
    StorageBackend: metrics.StorageDynamoDB,
    DynamoDBTable:  "custom-business-metrics-events",
    AWSRegion:      "us-east-1",
    DynamoEndpoint: "http://localhost:4566", // opcional, usado com LocalStack
    RetentionDays:  30,
})
```

O `Handler()` pode ser montado em um servidor existente, adapter Lambda, ALB, API Gateway, ECS ou
EKS.
