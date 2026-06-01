package com.tokensentinel.model;

public class ModelCost {
    private String model;
    private int totalTokens;
    private int totalInput;
    private int totalOutput;
    private int requestCount;
    private double avgInput;
    private double avgOutput;

    public String getModel() { return model; }
    public void setModel(String model) { this.model = model; }
    public int getTotalTokens() { return totalTokens; }
    public void setTotalTokens(int totalTokens) { this.totalTokens = totalTokens; }
    public int getTotalInput() { return totalInput; }
    public void setTotalInput(int totalInput) { this.totalInput = totalInput; }
    public int getTotalOutput() { return totalOutput; }
    public void setTotalOutput(int totalOutput) { this.totalOutput = totalOutput; }
    public int getRequestCount() { return requestCount; }
    public void setRequestCount(int requestCount) { this.requestCount = requestCount; }
    public double getAvgInput() { return avgInput; }
    public void setAvgInput(double avgInput) { this.avgInput = avgInput; }
    public double getAvgOutput() { return avgOutput; }
    public void setAvgOutput(double avgOutput) { this.avgOutput = avgOutput; }
}
