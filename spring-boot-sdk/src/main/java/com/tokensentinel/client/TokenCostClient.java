package com.tokensentinel.client;

import java.util.List;
import com.tokensentinel.model.ModelCost;
import com.tokensentinel.model.CostSummary;
import com.tokensentinel.model.AnomalyEntry;
import org.springframework.web.reactive.function.client.WebClient;
import reactor.core.publisher.Mono;
import reactor.util.retry.Retry;

import java.time.Duration;

public class TokenCostClient {
    private final WebClient webClient;

    public TokenCostClient(WebClient.Builder builder, String dashboardUrl, String apiKey, int connectTimeoutMs, int readTimeoutMs) {
        this.webClient = builder
            .baseUrl(dashboardUrl)
            .defaultHeader("Authorization", "Bearer " + apiKey)
            .build();
    }

    public List<ModelCost> getCostsByPeriod(String period) {
        return webClient.get()
            .uri(uri -> uri.path("/api/dashboard/costs").queryParam("period", period).build())
            .retrieve()
            .bodyToFlux(ModelCost.class)
            .collectList()
            .retryWhen(Retry.backoff(2, Duration.ofSeconds(1)))
            .block();
    }

    public CostSummary getSummary(String period) {
        return webClient.get()
            .uri(uri -> uri.path("/api/dashboard/summary").queryParam("period", period).build())
            .retrieve()
            .bodyToMono(CostSummary.class)
            .retryWhen(Retry.backoff(2, Duration.ofSeconds(1)))
            .block();
    }

    public List<AnomalyEntry> getAnomalies(String period) {
        return webClient.get()
            .uri(uri -> uri.path("/api/dashboard/anomalies").queryParam("period", period).build())
            .retrieve()
            .bodyToFlux(AnomalyEntry.class)
            .collectList()
            .retryWhen(Retry.backoff(2, Duration.ofSeconds(1)))
            .block();
    }
}
