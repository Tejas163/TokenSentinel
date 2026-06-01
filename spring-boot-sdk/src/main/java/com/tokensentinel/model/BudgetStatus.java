package com.tokensentinel.model;

public class BudgetStatus {
    private String team;
    private boolean budgeted;
    private long limit;
    private long used;
    private long remaining;
    private boolean overBudget;

    public String getTeam() { return team; }
    public void setTeam(String team) { this.team = team; }
    public boolean isBudgeted() { return budgeted; }
    public void setBudgeted(boolean budgeted) { this.budgeted = budgeted; }
    public long getLimit() { return limit; }
    public void setLimit(long limit) { this.limit = limit; }
    public long getUsed() { return used; }
    public void setUsed(long used) { this.used = used; }
    public long getRemaining() { return remaining; }
    public void setRemaining(long remaining) { this.remaining = remaining; }
    public boolean isOverBudget() { return overBudget; }
    public void setOverBudget(boolean overBudget) { this.overBudget = overBudget; }
}
