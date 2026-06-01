package com.tokensentinel.agent;

import com.tokensentinel.client.AdminClient;
import com.tokensentinel.client.TokenCostClient;
import com.tokensentinel.model.*;
import org.junit.jupiter.api.Test;
import org.springframework.web.reactive.function.client.WebClient;

import java.util.List;

import static org.junit.jupiter.api.Assertions.*;

class TokenSentinelAiToolsTest {
    private final TokenCostClient costClient;
    private final AdminClient adminClient;
    private final TokenSentinelAiTools tools;

    TokenSentinelAiToolsTest() {
        WebClient.Builder builder = WebClient.builder();
        this.costClient = new TokenCostClient(builder, "http://localhost:3001", "test-key", 5000, 30000);
        this.adminClient = new AdminClient(builder, "http://localhost:3001", "test-key");
        this.tools = new TokenSentinelAiTools(costClient, adminClient);
    }

    @Test
    void toolMethods_areAnnotated() throws Exception {
        var methods = TokenSentinelAiTools.class.getDeclaredMethods();
        assertTrue(methods.length >= 10, "Expected at least 10 @Tool methods");

        for (var m : methods) {
            var toolAnn = m.getAnnotation(org.springframework.ai.tool.annotation.Tool.class);
            assertNotNull(toolAnn, "Method " + m.getName() + " is missing @Tool annotation");
            assertFalse(toolAnn.name().isEmpty(), "Method " + m.getName() + " has empty tool name");
            assertFalse(toolAnn.description().isEmpty(), "Method " + m.getName() + " has empty description");
        }
    }

    @Test
    void toolMethods_haveCorrectReturnTypes() throws Exception {
        assertEquals(CostSummary.class,
            TokenSentinelAiTools.class.getMethod("getCostSummary", String.class).getReturnType());
        assertEquals(List.class,
            TokenSentinelAiTools.class.getMethod("getModelCosts", String.class).getReturnType());
        assertEquals(List.class,
            TokenSentinelAiTools.class.getMethod("getAnomalies", String.class).getReturnType());
        assertEquals(List.class,
            TokenSentinelAiTools.class.getMethod("listTeams").getReturnType());
        assertEquals(Team.class,
            TokenSentinelAiTools.class.getMethod("createTeam", String.class, long.class, String.class).getReturnType());
        assertEquals(void.class,
            TokenSentinelAiTools.class.getMethod("deleteTeam", int.class).getReturnType());
        assertEquals(BudgetStatus.class,
            TokenSentinelAiTools.class.getMethod("getBudgetStatus", String.class).getReturnType());
        assertEquals(List.class,
            TokenSentinelAiTools.class.getMethod("listBudgetRules").getReturnType());
        assertEquals(BudgetRule.class,
            TokenSentinelAiTools.class.getMethod("createBudgetRule", String.class, long.class, String.class, String.class).getReturnType());
        assertEquals(void.class,
            TokenSentinelAiTools.class.getMethod("deleteBudgetRule", int.class).getReturnType());
    }

    @Test
    void toolDescriptions_includeUsageExamples() throws Exception {
        var createTeam = TokenSentinelAiTools.class.getMethod("createTeam", String.class, long.class, String.class);
        var ann = createTeam.getAnnotation(org.springframework.ai.tool.annotation.Tool.class);
        assertTrue(ann.description().contains("engineering"), "Description should include usage example");
    }
}
