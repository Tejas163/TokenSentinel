package com.tokensentinel.client;

import com.tokensentinel.model.Team;
import com.tokensentinel.model.BudgetRule;
import com.tokensentinel.model.BudgetStatus;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.web.reactive.function.client.WebClient;

import static org.junit.jupiter.api.Assertions.*;

class AdminClientTest {
    private MockWebServer server;
    private AdminClient client;

    @BeforeEach
    void setUp() {
        server = new MockWebServer();
        client = new AdminClient(WebClient.builder(), server.url("/").toString(), "test-key");
    }

    @AfterEach
    void tearDown() throws Exception {
        server.shutdown();
    }

    @Test
    void getTeams_returnsTeamList() throws Exception {
        server.enqueue(new MockResponse()
            .setBody("[{\"id\":1,\"name\":\"engineering\",\"monthly_token_budget\":10000000,\"period\":\"30d\"}]")
            .addHeader("Content-Type", "application/json"));

        var teams = client.getTeams();

        assertEquals(1, teams.size());
        assertEquals("engineering", teams.get(0).getName());
        assertEquals(10000000, teams.get(0).getMonthlyTokenBudget());
    }

    @Test
    void createTeam_postsAndReturnsTeam() throws Exception {
        server.enqueue(new MockResponse()
            .setBody("{\"id\":2,\"name\":\"ml-research\",\"monthly_token_budget\":5000000,\"period\":\"30d\"}")
            .addHeader("Content-Type", "application/json"));

        Team created = client.createTeam("ml-research", 5000000);

        assertEquals("ml-research", created.getName());
        assertEquals(5000000, created.getMonthlyTokenBudget());
    }

    @Test
    void createTeam_withCustomPeriod() throws Exception {
        server.enqueue(new MockResponse()
            .setBody("{\"id\":3,\"name\":\"qa\",\"monthly_token_budget\":2000000,\"period\":\"7d\"}")
            .addHeader("Content-Type", "application/json"));

        Team created = client.createTeam("qa", 2000000, "7d");

        assertEquals("7d", created.getPeriod());
    }

    @Test
    void deleteTeam_sendsDeleteRequest() throws Exception {
        server.enqueue(new MockResponse().setResponseCode(204));

        client.deleteTeam(1);

        var request = server.takeRequest();
        assertEquals("DELETE", request.getMethod());
        assertTrue(request.getPath().contains("id=1"));
    }

    @Test
    void getBudgetRules_returnsRules() throws Exception {
        server.enqueue(new MockResponse()
            .setBody("[{\"id\":1,\"model\":\"gpt-4\",\"max_tokens\":5000000,\"period\":\"24h\",\"webhook_url\":\"https://hooks.example.com\",\"enabled\":true}]")
            .addHeader("Content-Type", "application/json"));

        var rules = client.getBudgetRules();

        assertEquals(1, rules.size());
        assertEquals("gpt-4", rules.get(0).getModel());
        assertEquals(5000000, rules.get(0).getMaxTokens());
        assertTrue(rules.get(0).isEnabled());
    }

    @Test
    void createBudgetRule_postsAndReturns() throws Exception {
        server.enqueue(new MockResponse()
            .setBody("{\"id\":2,\"model\":\"gpt-3.5-turbo\",\"max_tokens\":2000000,\"period\":\"24h\",\"webhook_url\":\"https://hooks.example.com\",\"enabled\":true}")
            .addHeader("Content-Type", "application/json"));

        BudgetRule rule = client.createBudgetRule("gpt-3.5-turbo", 2000000, "https://hooks.example.com");

        assertEquals("gpt-3.5-turbo", rule.getModel());
        assertEquals(2000000, rule.getMaxTokens());
    }

    @Test
    void deleteBudgetRule_sendsDelete() throws Exception {
        server.enqueue(new MockResponse().setResponseCode(204));

        client.deleteBudgetRule(1);

        var request = server.takeRequest();
        assertEquals("DELETE", request.getMethod());
    }

    @Test
    void getBudgetStatus_returnsStatus() throws Exception {
        server.enqueue(new MockResponse()
            .setBody("{\"team\":\"engineering\",\"budgeted\":true,\"limit\":10000000,\"used\":3420000,\"remaining\":6580000,\"over_budget\":false}")
            .addHeader("Content-Type", "application/json"));

        BudgetStatus status = client.getBudgetStatus("engineering");

        assertTrue(status.isBudgeted());
        assertEquals(10000000, status.getLimit());
        assertEquals(3420000, status.getUsed());
        assertEquals(6580000, status.getRemaining());
        assertFalse(status.isOverBudget());
    }

    @Test
    void allEndpoints_sendAuthHeader() throws Exception {
        server.enqueue(new MockResponse()
            .setBody("[]")
            .addHeader("Content-Type", "application/json"));

        client.getTeams();

        var request = server.takeRequest();
        assertEquals("Bearer test-key", request.getHeader("Authorization"));
    }
}
