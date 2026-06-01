package com.tokensentinel.client;

import java.util.List;
import com.tokensentinel.model.ModelCost;
import com.tokensentinel.model.CostSummary;
import org.springframework.web.reactive.function.client.WebClient;
import reactor.core.publisher.Mono;

public class TokenCostClient {
    private final WebClient webClient;

    public TokenCostClient(WebClient.Builder builder, String dashboardUrl, String apiKey) {
        this.webClient = builder.baseUrl(dashboardUrl)
            .defaultHeader("Authorization", "Bearer " + apiKey)
            .build();
    }

    public List<ModelCost> getCostsByPeriod(String period) {
        return webClient.get()
            .uri(uri -> uri.path("/api/dashboard/costs").queryParam("period", period).build())
            .retrieve()
            .bodyToFlux(ModelCost.class)
            .collectList()
            .block();
    }

    public CostSummary getSummary(String period) {
        return webClient.get()
            .uri(uri -> uri.path("/api/dashboard/summary").queryParam("period", period).build())
            .retrieve()
            .bodyToMono(CostSummary.class)
            .block();
    }
}
