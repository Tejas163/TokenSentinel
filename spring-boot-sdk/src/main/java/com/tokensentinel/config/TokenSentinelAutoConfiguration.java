package com.tokensentinel.config;

import com.tokensentinel.agent.TokenSentinelAiTools;
import com.tokensentinel.client.TokenCostClient;
import com.tokensentinel.client.AdminClient;

import io.micrometer.core.instrument.MeterRegistry;
import org.springframework.ai.tool.ToolCallback;
import org.springframework.ai.tool.method.MethodToolCallbackProvider;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.reactive.function.client.WebClient;

import java.util.List;

@Configuration
@EnableConfigurationProperties(TokenSentinelProperties.class)
public class TokenSentinelAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public TokenCostClient tokenCostClient(WebClient.Builder builder, TokenSentinelProperties props) {
        return new TokenCostClient(builder, props.getDashboardUrl(), props.getApiKey(),
            props.getConnectTimeoutMs(), props.getReadTimeoutMs());
    }

    @Bean
    @ConditionalOnMissingBean
    public AdminClient adminClient(WebClient.Builder builder, TokenSentinelProperties props) {
        return new AdminClient(builder, props.getDashboardUrl(), props.getApiKey());
    }

    @Bean
    @ConditionalOnMissingBean
    public TokenSentinelMetrics tokenSentinelMetrics(MeterRegistry registry) {
        return new TokenSentinelMetrics(registry);
    }

    @Bean
    @ConditionalOnMissingBean
    public TokenSentinelAiTools tokenSentinelAiTools(TokenCostClient costClient, AdminClient adminClient) {
        return new TokenSentinelAiTools(costClient, adminClient);
    }

    @Bean
    public List<ToolCallback> tokenSentinelToolCallbacks(TokenSentinelAiTools tools) {
        return MethodToolCallbackProvider.builder()
            .toolObjects(tools)
            .build()
            .getToolCallbacks();
    }
}
