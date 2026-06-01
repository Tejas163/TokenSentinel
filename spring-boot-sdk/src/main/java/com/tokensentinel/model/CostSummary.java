package com.tokensentinel.model;

public class CostSummary {
    private int totalRequests;
    private int totalTokens;
    private int totalInput;
    private int totalOutput;
    private int uniqueModels;
    private String period;
    private double avgTokensPerRequest;

    public int getTotalRequests() { return totalRequests; }
    public void setTotalRequests(int totalRequests) { this.totalRequests = totalRequests; }
    public int getTotalTokens() { return totalTokens; }
    public void setTotalTokens(int totalTokens) { this.totalTokens = totalTokens; }
    public int getTotalInput() { return totalInput; }
    public void setTotalInput(int totalInput) { this.totalInput = totalInput; }
    public int getTotalOutput() { return totalOutput; }
    public void setTotalOutput(int totalOutput) { this.totalOutput = totalOutput; }
    public int getUniqueModels() { return uniqueModels; }
    public void setUniqueModels(int uniqueModels) { this.uniqueModels = uniqueModels; }
    public String getPeriod() { return period; }
    public void setPeriod(String period) { this.period = period; }
    public double getAvgTokensPerRequest() { return avgTokensPerRequest; }
    public void setAvgTokensPerRequest(double avgTokensPerRequest) { this.avgTokensPerRequest = avgTokensPerRequest; }
}
