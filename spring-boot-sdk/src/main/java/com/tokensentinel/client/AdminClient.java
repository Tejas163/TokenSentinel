package com.tokensentinel.client;

import org.springframework.web.reactive.function.client.WebClient;

public class AdminClient {
    private final WebClient webClient;

    public AdminClient(WebClient.Builder builder, String gatewayUrl, String apiKey) {
        this.webClient = builder.baseUrl(gatewayUrl)
            .defaultHeader("Authorization", "Bearer " + apiKey)
            .build();
    }

    public void registerRoute(String path, String routeConfigJson) {
        webClient.post()
            .uri("/admin/routes")
            .bodyValue(routeConfigJson)
            .retrieve()
            .toBodilessEntity()
            .block();
    }

    public String getAlerts() {
        return webClient.get()
            .uri("/admin/alerts")
            .retrieve()
            .bodyToMono(String.class)
            .block();
    }
}
