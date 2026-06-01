package com.tokensentinel.model;

public class AnomalyEntry {
    private String requestId;
    private String model;
    private int totalTokens;
    private double mean;
    private double stddev;
    private double zScore;

    public String getRequestId() { return requestId; }
    public void setRequestId(String requestId) { this.requestId = requestId; }
    public String getModel() { return model; }
    public void setModel(String model) { this.model = model; }
    public int getTotalTokens() { return totalTokens; }
    public void setTotalTokens(int totalTokens) { this.totalTokens = totalTokens; }
    public double getMean() { return mean; }
    public void setMean(double mean) { this.mean = mean; }
    public double getStddev() { return stddev; }
    public void setStddev(double stddev) { this.stddev = stddev; }
    public double getZScore() { return zScore; }
    public void setZScore(double zScore) { this.zScore = zScore; }
}
