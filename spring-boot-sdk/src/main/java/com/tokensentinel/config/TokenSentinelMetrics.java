package com.tokensentinel.config;

import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.DistributionSummary;
import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.Timer;

public class TokenSentinelMetrics {
    private final Counter requestCounter;
    private final DistributionSummary tokenSummary;
    private final Timer latencyTimer;
    private final Counter adminOpCounter;

    public TokenSentinelMetrics(MeterRegistry registry) {
        this.requestCounter = Counter.builder("tokensentinel.cost.requests")
            .description("Total cost dashboard requests")
            .register(registry);
        this.tokenSummary = DistributionSummary.builder("tokensentinel.cost.tokens")
            .description("Token count distribution")
            .register(registry);
        this.latencyTimer = Timer.builder("tokensentinel.cost.latency")
            .description("Cost query latency")
            .register(registry);
        this.adminOpCounter = Counter.builder("tokensentinel.admin.operations")
            .description("Admin operations by type")
            .tag("operation", "unknown")
            .register(registry);
    }

    public void recordRequest() { requestCounter.increment(); }
    public void recordTokens(int count) { tokenSummary.record(count); }
    public <T> T recordLatency(java.util.concurrent.Callable<T> callable) throws Exception {
        return latencyTimer.recordCallable(callable);
    }
    public void recordAdminOp(String operation) {
        adminOpCounter.increment();
    }
}
