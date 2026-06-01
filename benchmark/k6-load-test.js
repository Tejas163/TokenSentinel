import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('errors');
const latencyTrend = new Trend('latency_ms');

export const options = {
  stages: [
    { duration: '30s', target: 10 },
    { duration: '1m', target: 50 },
    { duration: '30s', target: 100 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    errors: ['rate<0.05'],
    http_req_duration: ['p(95)<2000'],
  },
};

const GATEWAY_URL = __ENV.GATEWAY_URL || 'http://localhost:8080';
const DASHBOARD_URL = __ENV.DASHBOARD_URL || 'http://localhost:3001';

export default function () {
  const res = http.get(`${GATEWAY_URL}/health`);
  check(res, {
    'health status is 200': (r) => r.status === 200,
  });
  latencyTrend.add(res.timings.duration);
  errorRate.add(res.status !== 200);

  const costRes = http.get(`${DASHBOARD_URL}/api/dashboard/summary?period=1h`);
  check(costRes, {
    'cost summary is 200': (r) => r.status === 200,
    'cost summary has total_requests': (r) => r.json('total_requests') !== undefined,
  });

  const modelCostRes = http.get(`${DASHBOARD_URL}/api/dashboard/costs?period=1h`);
  check(modelCostRes, {
    'cost by model is 200': (r) => r.status === 200,
  });

  sleep(1);
}

export function handleSummary(data) {
  return {
    'benchmark/results.json': JSON.stringify({
      metrics: {
        avg_latency_ms: data.metrics.latency_ms.values.avg,
        p95_latency_ms: data.metrics.latency_ms.values['p(95)'],
        error_rate: data.metrics.errors.values.rate,
        total_requests: data.metrics.http_reqs.values.count,
      },
      thresholds_met: Object.entries(data.metrics).every(
        ([, m]) => !m.thresholds || Object.values(m.thresholds).every((t) => t.ok)
      ),
    }),
  };
}
