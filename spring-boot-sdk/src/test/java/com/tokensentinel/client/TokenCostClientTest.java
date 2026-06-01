package com.tokensentinel.client;

import com.tokensentinel.model.ModelCost;
import com.tokensentinel.model.CostSummary;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.web.reactive.function.client.WebClient;

import java.util.List;

import static org.junit.jupiter.api.Assertions.*;

class TokenCostClientTest {
    private MockWebServer server;
    private TokenCostClient client;

    @BeforeEach
    void setUp() {
        server = new MockWebServer();
        client = new TokenCostClient(WebClient.builder(), server.url("/").toString(), "test-key", 5000, 30000);
    }

    @AfterEach
    void tearDown() throws Exception {
        server.shutdown();
    }

    @Test
    void getCostsByPeriod_returnsModelCosts() throws Exception {
        server.enqueue(new MockResponse()
            .setBody("[{\"model\":\"gpt-4\",\"total_tokens\":1000,\"total_input\":400,\"total_output\":600,\"request_count\":5,\"avg_input\":80.0,\"avg_output\":120.0}]")
            .addHeader("Content-Type", "application/json"));

        List<ModelCost> costs = client.getCostsByPeriod("24h");

        assertEquals(1, costs.size());
        assertEquals("gpt-4", costs.get(0).getModel());
        assertEquals(1000, costs.get(0).getTotalTokens());
        assertEquals(5, costs.get(0).getRequestCount());
    }

    @Test
    void getCostsByPeriod_passesPeriodParam() throws Exception {
        server.enqueue(new MockResponse()
            .setBody("[]")
            .addHeader("Content-Type", "application/json"));

        client.getCostsByPeriod("168h");

        var request = server.takeRequest();
        assertTrue(request.getPath().contains("period=168h"));
    }

    @Test
    void getSummary_returnsCostSummary() throws Exception {
        server.enqueue(new MockResponse()
            .setBody("{\"total_requests\":10,\"total_tokens\":5000,\"total_input\":2000,\"total_output\":3000,\"unique_models\":3,\"period\":\"24h\",\"avg_tokens_per_request\":500.0}")
            .addHeader("Content-Type", "application/json"));

        CostSummary summary = client.getSummary("24h");

        assertEquals(10, summary.getTotalRequests());
        assertEquals(5000, summary.getTotalTokens());
        assertEquals(3, summary.getUniqueModels());
        assertEquals(500.0, summary.getAvgTokensPerRequest(), 0.01);
    }

    @Test
    void getAnomalies_returnsAnomalyEntries() throws Exception {
        server.enqueue(new MockResponse()
            .setBody("[{\"request_id\":\"abc-123\",\"model\":\"gpt-4\",\"total_tokens\":5000,\"mean\":1000.0,\"stddev\":200.0,\"z_score\":20.0}]")
            .addHeader("Content-Type", "application/json"));

        var anomalies = client.getAnomalies("24h");

        assertEquals(1, anomalies.size());
        assertEquals("gpt-4", anomalies.get(0).getModel());
        assertEquals(5000, anomalies.get(0).getTotalTokens());
        assertEquals(20.0, anomalies.get(0).getZScore(), 0.01);
    }

    @Test
    void getSummary_sendsAuthHeader() throws Exception {
        server.enqueue(new MockResponse()
            .setBody("{\"total_requests\":0}")
            .addHeader("Content-Type", "application/json"));

        client.getSummary("24h");

        var request = server.takeRequest();
        assertEquals("Bearer test-key", request.getHeader("Authorization"));
    }
}
