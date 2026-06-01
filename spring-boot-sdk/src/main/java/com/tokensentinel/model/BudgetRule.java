package com.tokensentinel.model;

public class BudgetRule {
    private int id;
    private String model;
    private long maxTokens;
    private String period;
    private String webhookUrl;
    private boolean enabled;

    public int getId() { return id; }
    public void setId(int id) { this.id = id; }
    public String getModel() { return model; }
    public void setModel(String model) { this.model = model; }
    public long getMaxTokens() { return maxTokens; }
    public void setMaxTokens(long maxTokens) { this.maxTokens = maxTokens; }
    public String getPeriod() { return period; }
    public void setPeriod(String period) { this.period = period; }
    public String getWebhookUrl() { return webhookUrl; }
    public void setWebhookUrl(String webhookUrl) { this.webhookUrl = webhookUrl; }
    public boolean isEnabled() { return enabled; }
    public void setEnabled(boolean enabled) { this.enabled = enabled; }
}
