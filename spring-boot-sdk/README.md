# TokenSentinel Enterprise SDK (Spring AI)

Spring Boot Starter that exposes the TokenSentinel cost dashboard as **Spring AI tools**.
Any `ChatClient` in your application can call these tools to query costs, detect anomalies,
manage teams, and check budgets — all through natural language.

## Quick Start

```xml
<dependency>
    <groupId>com.tokensentinel</groupId>
    <artifactId>token-sentinel-sdk</artifactId>
    <version>0.1.0</version>
</dependency>
```

```yaml
tokensentinel:
  dashboard-url: http://localhost:3001
  api-key: ${TOKENSENTINEL_API_KEY}
```

## Usage: ChatClient with tools

```java
@Autowired ChatClient.Builder chatClientBuilder;

public void ask() {
    ChatClient client = chatClientBuilder.build();

    String answer = client.prompt("How many tokens did gpt-4 use in the last 24h?")
        .call()
        .content();
    // The LLM calls get_model_costs("24h"), picks gpt-4's total_tokens, and answers.
}
```

The SDK auto-registers all `TokenSentinelAiTools` methods as `ToolCallback` beans.
Spring AI's `ChatClient.Builder` picks them up automatically when you call `.build()`.

## Available Tools

| Tool | What it does | Returns |
|------|-------------|---------|
| `get_cost_summary` | Aggregate cost over a period | CostSummary |
| `get_model_costs` | Per-model token breakdown | List\<ModelCost\> |
| `get_anomalies` | 3σ anomaly detection | List\<AnomalyEntry\> |
| `list_teams` | All registered teams | List\<Team\> |
| `create_team` | Add a team with budget | Team |
| `delete_team` | Remove a team | void |
| `get_budget_status` | Over/under budget check | BudgetStatus |
| `list_budget_rules` | Budget alert rules | List\<BudgetRule\> |
| `create_budget_rule` | Add a budget alert | BudgetRule |
| `delete_budget_rule` | Remove a budget alert | void |

All tools accept `period` values: `1h`, `6h`, `24h`, `72h`, `168h`

## Direct REST Clients

If you need the raw API without Spring AI:

```java
@Autowired TokenCostClient costClient;
CostSummary summary = costClient.getSummary("24h");
List<AnomalyEntry> anomalies = costClient.getAnomalies("24h");

@Autowired AdminClient adminClient;
List<Team> teams = adminClient.getTeams();
BudgetStatus status = adminClient.getBudgetStatus("engineering");
```

## Models

| Class | Key Fields |
|-------|------------|
| `CostSummary` | totalRequests, totalTokens, uniqueModels, avgTokensPerRequest |
| `ModelCost` | model, totalTokens, requestCount, avgInput, avgOutput |
| `AnomalyEntry` | model, totalTokens, mean, stddev, zScore |
| `Team` | name, monthlyTokenBudget, period |
| `BudgetRule` | model, maxTokens, period, webhookUrl, enabled |
| `BudgetStatus` | team, limit, used, remaining, overBudget |

## Metrics

Meters auto-register when actuator is on the classpath:

| Meter | Type |
|-------|------|
| `tokensentinel.cost.requests` | Counter |
| `tokensentinel.cost.tokens` | DistributionSummary |
| `tokensentinel.cost.latency` | Timer |
| `tokensentinel.admin.operations` | Counter (tagged by operation) |

## Building

```bash
mvn clean install
mvn test
```
