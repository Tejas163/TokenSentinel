package com.tokensentinel.client;

import com.tokensentinel.model.Team;
import com.tokensentinel.model.BudgetRule;
import com.tokensentinel.model.BudgetStatus;
import reactor.util.retry.Retry;

import org.springframework.web.reactive.function.client.WebClient;
import java.time.Duration;
import java.util.List;

public class AdminClient {
    private final WebClient dashboardClient;
    private final String apiKey;

    public AdminClient(WebClient.Builder builder, String dashboardUrl, String apiKey) {
        this.dashboardClient = builder
            .baseUrl(dashboardUrl)
            .defaultHeader("Authorization", "Bearer " + apiKey)
            .build();
        this.apiKey = apiKey;
    }

    public List<Team> getTeams() {
        return dashboardClient.get()
            .uri("/api/admin/teams")
            .retrieve()
            .bodyToFlux(Team.class)
            .collectList()
            .retryWhen(Retry.backoff(2, Duration.ofSeconds(1)))
            .block();
    }

    public Team createTeam(String name, long monthlyTokenBudget) {
        return createTeam(name, monthlyTokenBudget, "30d");
    }

    public Team createTeam(String name, long monthlyTokenBudget, String period) {
        Team body = new Team();
        body.setName(name);
        body.setMonthlyTokenBudget(monthlyTokenBudget);
        body.setPeriod(period);
        return dashboardClient.post()
            .uri("/api/admin/teams")
            .bodyValue(body)
            .retrieve()
            .bodyToMono(Team.class)
            .retryWhen(Retry.backoff(2, Duration.ofSeconds(1)))
            .block();
    }

    public void deleteTeam(int id) {
        dashboardClient.delete()
            .uri(uri -> uri.path("/api/admin/teams").queryParam("id", id).build())
            .retrieve()
            .toBodilessEntity()
            .retryWhen(Retry.backoff(2, Duration.ofSeconds(1)))
            .block();
    }

    public List<BudgetRule> getBudgetRules() {
        return dashboardClient.get()
            .uri("/api/admin/budget-rules")
            .retrieve()
            .bodyToFlux(BudgetRule.class)
            .collectList()
            .retryWhen(Retry.backoff(2, Duration.ofSeconds(1)))
            .block();
    }

    public BudgetRule createBudgetRule(String model, long maxTokens, String webhookUrl) {
        return createBudgetRule(model, maxTokens, "24h", webhookUrl);
    }

    public BudgetRule createBudgetRule(String model, long maxTokens, String period, String webhookUrl) {
        BudgetRule body = new BudgetRule();
        body.setModel(model);
        body.setMaxTokens(maxTokens);
        body.setPeriod(period);
        body.setWebhookUrl(webhookUrl);
        return dashboardClient.post()
            .uri("/api/admin/budget-rules")
            .bodyValue(body)
            .retrieve()
            .bodyToMono(BudgetRule.class)
            .retryWhen(Retry.backoff(2, Duration.ofSeconds(1)))
            .block();
    }

    public void deleteBudgetRule(int id) {
        dashboardClient.delete()
            .uri(uri -> uri.path("/api/admin/budget-rules").queryParam("id", id).build())
            .retrieve()
            .toBodilessEntity()
            .retryWhen(Retry.backoff(2, Duration.ofSeconds(1)))
            .block();
    }

    public BudgetStatus getBudgetStatus(String team) {
        return dashboardClient.get()
            .uri(uri -> uri.path("/api/budget/status").queryParam("team", team).build())
            .retrieve()
            .bodyToMono(BudgetStatus.class)
            .retryWhen(Retry.backoff(2, Duration.ofSeconds(1)))
            .block();
    }
}
