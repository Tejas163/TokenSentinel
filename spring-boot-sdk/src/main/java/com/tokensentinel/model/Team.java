package com.tokensentinel.model;

public class Team {
    private int id;
    private String name;
    private long monthlyTokenBudget;
    private String period;

    public int getId() { return id; }
    public void setId(int id) { this.id = id; }
    public String getName() { return name; }
    public void setName(String name) { this.name = name; }
    public long getMonthlyTokenBudget() { return monthlyTokenBudget; }
    public void setMonthlyTokenBudget(long monthlyTokenBudget) { this.monthlyTokenBudget = monthlyTokenBudget; }
    public String getPeriod() { return period; }
    public void setPeriod(String period) { this.period = period; }
}
