package com.tokensentinel.config;

import com.tokensentinel.client.TokenCostClient;
import com.tokensentinel.client.AdminClient;
import io.micrometer.core.instrument.MeterRegistry;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.reactive.function.client.WebClient;

@Configuration
@EnableConfigurationProperties(TokenSentinelProperties.class)
public class TokenSentinelAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public TokenCostClient tokenCostClient(WebClient.Builder builder, TokenSentinelProperties props) {
        return new TokenCostClient(builder, props.getDashboardUrl(), props.getApiKey());
    }

    @Bean
    @ConditionalOnMissingBean
    public AdminClient adminClient(WebClient.Builder builder, TokenSentinelProperties props) {
        return new AdminClient(builder, props.getGatewayUrl(), props.getApiKey());
    }

    @Bean
    public TokenSentinelMetrics tokenSentinelMetrics(MeterRegistry registry) {
        return new TokenSentinelMetrics(registry);
    }
}
