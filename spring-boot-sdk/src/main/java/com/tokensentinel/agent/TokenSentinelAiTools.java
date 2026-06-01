package com.tokensentinel.agent;

import com.tokensentinel.client.AdminClient;
import com.tokensentinel.client.TokenCostClient;
import com.tokensentinel.model.*;

import org.springframework.ai.tool.annotation.Tool;
import org.springframework.stereotype.Component;

import java.util.List;

@Component
public class TokenSentinelAiTools {
    private final TokenCostClient costClient;
    private final AdminClient adminClient;

    public TokenSentinelAiTools(TokenCostClient costClient, AdminClient adminClient) {
        this.costClient = costClient;
        this.adminClient = adminClient;
    }

    @Tool(name = "get_cost_summary",
          description = "Get aggregate token cost summary for a time period. " +
                        "Period values: 1h, 6h, 24h, 72h, 168h. Returns total requests, tokens, and model count.")
    public CostSummary getCostSummary(String period) {
        return costClient.getSummary(period);
    }

    @Tool(name = "get_model_costs",
          description = "Get per-model token cost breakdown for a time period. " +
                        "Period values: 1h, 6h, 24h, 72h, 168h. Each entry shows model, tokens, and request count.")
    public List<ModelCost> getModelCosts(String period) {
        return costClient.getCostsByPeriod(period);
    }

    @Tool(name = "get_anomalies",
          description = "Detect anomalous token usage using 3-sigma outlier detection. " +
                        "Flags requests exceeding mean + 3 standard deviations for their model. " +
                        "Period values: 1h, 6h, 24h, 72h, 168h.")
    public List<AnomalyEntry> getAnomalies(String period) {
        return costClient.getAnomalies(period);
    }

    @Tool(name = "list_teams",
          description = "List all teams registered in TokenSentinel with their monthly token budgets and periods.")
    public List<Team> listTeams() {
        return adminClient.getTeams();
    }

    @Tool(name = "create_team",
          description = "Register a new team with a monthly token budget for budget-aware routing. " +
                        "Example: createTeam('engineering', 10000000, '30d'). Period defaults to 30d.")
    public Team createTeam(String name, long monthlyTokenBudget, String period) {
        return adminClient.createTeam(name, monthlyTokenBudget, period);
    }

    @Tool(name = "delete_team",
          description = "Remove a team by its numeric ID.")
    public void deleteTeam(int id) {
        adminClient.deleteTeam(id);
    }

    @Tool(name = "get_budget_status",
          description = "Check whether a team is over or under its monthly token budget. " +
                        "Returns limit, used, remaining, and overBudget flag.")
    public BudgetStatus getBudgetStatus(String team) {
        return adminClient.getBudgetStatus(team);
    }

    @Tool(name = "list_budget_rules",
          description = "List all budget threshold alert rules. Each rule fires a webhook when " +
                        "a model exceeds max_tokens within its period.")
    public List<BudgetRule> listBudgetRules() {
        return adminClient.getBudgetRules();
    }

    @Tool(name = "create_budget_rule",
          description = "Create a budget threshold alert rule. Fires a webhook when a model " +
                        "exceeds max_tokens within the period. Example: " +
                        "createBudgetRule('gpt-4', 5000000, '24h', 'https://hooks.slack.com/...')")
    public BudgetRule createBudgetRule(String model, long maxTokens, String period, String webhookUrl) {
        return adminClient.createBudgetRule(model, maxTokens, period, webhookUrl);
    }

    @Tool(name = "delete_budget_rule",
          description = "Remove a budget rule by its numeric ID.")
    public void deleteBudgetRule(int id) {
        adminClient.deleteBudgetRule(id);
    }
}
