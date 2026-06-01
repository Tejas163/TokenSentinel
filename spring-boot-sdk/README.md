# TokenSentinel Enterprise SDK (Spring Boot)

## Overview
Auto-configured Spring Boot Starter for integrating enterprise services with TokenSentinel.
Provides typed clients for cost queries, route management, budget administration, and
Micrometer metrics bridge.

## Quick Start

Add dependency to your `pom.xml`:

```xml
<dependency>
    <groupId>com.tokensentinel</groupId>
    <artifactId>token-sentinel-sdk</artifactId>
    <version>0.1.0</version>
</dependency>
```

Configure in `application.yml`:

```yaml
tokensentinel:
  gateway-url: http://localhost:8080
  dashboard-url: http://localhost:3001
  api-key: ${TOKENSENTINEL_API_KEY}
```

## Clients

### TokenCostClient
Query cost data from the TokenSentinel dashboard:

```java
@Autowired TokenCostClient costClient;

// Cost breakdown by model (last 24h)
List<ModelCost> costs = costClient.getCostsByPeriod("24h");

// Aggregate summary
CostSummary summary = costClient.getSummary("24h");
```

### AdminClient
Manage routes, providers, and budgets:

```java
@Autowired AdminClient adminClient;

// Register a new route
adminClient.registerRoute("/v1/chat/completions", providers);

// Get active budget alerts
List<BudgetAlert> alerts = adminClient.getAlerts();
```

## Metrics
The SDK exposes Micrometer meters automatically:
- `tokensentinel.cost.requests` — counter
- `tokensentinel.cost.tokens` — distribution summary
- `tokensentinel.cost.latency` — timer
- `tokensentinel.admin.operations` — counter by operation type

## Building

```bash
mvn clean package -DskipTests
```
