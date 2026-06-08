import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('errors');
const latencyTrend = new Trend('latency_ms');

const GATEWAY_URL = __ENV.GATEWAY_URL || 'http://localhost:8080';
const DASHBOARD_URL = __ENV.DASHBOARD_URL || 'http://localhost:3001';

export const options = {
  scenarios: {
    ramp_up: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 20 },
        { duration: '30s', target: 50 },
        { duration: '30s', target: 100 },
        { duration: '30s', target: 0 },
      ],
      tags: { scenario: 'ramp_up' },
      gracefulStop: '10s',
    },
    soak: {
      executor: 'constant-vus',
      vus: 20,
      duration: '2m',
      startTime: '2m30s',
      tags: { scenario: 'soak' },
      gracefulStop: '10s',
    },
    spike: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '10s', target: 200 },
        { duration: '30s', target: 200 },
        { duration: '10s', target: 0 },
      ],
      startTime: '5m',
      tags: { scenario: 'spike' },
      gracefulStop: '10s',
    },
  },
  thresholds: {
    errors: ['rate<0.05'],
    http_req_duration: ['p(95)<2000', 'p(99)<5000'],
    'http_req_duration{scenario:ramp_up}': ['p(95)<1500'],
    'http_req_duration{scenario:soak}': ['p(95)<2000'],
    'http_req_duration{scenario:spike}': ['p(95)<5000'],
  },
};

function checkHealth() {
  group('health checks', () => {
    const res = http.get(`${GATEWAY_URL}/health`);
    check(res, { 'gateway health is 200': (r) => r.status === 200 });
    latencyTrend.add(res.timings.duration);
    errorRate.add(res.status !== 200);

    const resAll = http.get(`${DASHBOARD_URL}/health`);
    check(resAll, { 'dashboard health is 200': (r) => r.status === 200 });
  });
}

function costEndpoints() {
  group('cost endpoints', () => {
    const summary = http.get(`${DASHBOARD_URL}/api/dashboard/summary?period=1h`);
    check(summary, {
      'summary is 200': (r) => r.status === 200,
      'summary has data': (r) => r.json('total_requests') !== undefined,
    });
    errorRate.add(summary.status !== 200);

    const costs = http.get(`${DASHBOARD_URL}/api/dashboard/costs?period=1h`);
    check(costs, { 'costs is 200': (r) => r.status === 200 });

    const ts = http.get(`${DASHBOARD_URL}/api/dashboard/cost-timeseries?period=1h`);
    check(ts, { 'timeseries is 200': (r) => r.status === 200 });
  });
}

function anomalies() {
  group('anomalies', () => {
    const res = http.get(`${DASHBOARD_URL}/api/dashboard/anomalies?period=168h`);
    check(res, { 'anomalies is 200': (r) => r.status === 200 });
    errorRate.add(res.status !== 200);
  });
}

function prescriptive() {
  group('prescriptive', () => {
    const res = http.get(`${DASHBOARD_URL}/api/prescriptive/models`);
    check(res, { 'prescriptive models is 200': (r) => r.status === 200 });
  });
}

function budgetStatus() {
  group('budget', () => {
    const res = http.get(`${DASHBOARD_URL}/api/budget/status?team=engineering`);
    check(res, { 'budget status is 200': (r) => r.status === 200 });
  });
}

export default function () {
  checkHealth();
  sleep(0.5);
  costEndpoints();
  sleep(0.5);
  anomalies();
  sleep(0.3);
  prescriptive();
  sleep(0.3);
  budgetStatus();
  sleep(0.4);
}

export function handleSummary(data) {
  const extract = (scenario, metric) => {
    const byScenario = data.metrics[metric]?.valuesByScenario?.[scenario];
    return byScenario || data.metrics[metric]?.values;
  };

  const scenarios = ['ramp_up', 'soak', 'spike'];
  const scenarioResults = {};
  for (const s of scenarios) {
    const vals = extract(s, 'http_req_duration');
    scenarioResults[s] = vals ? {
      avg_latency_ms: vals.avg,
      p95_latency_ms: vals['p(95)'],
      p99_latency_ms: vals['p(99)'],
      error_rate: extract(s, 'errors')?.rate ?? 0,
    } : { error: 'no data' };
  }

  return {
    'benchmark/results.json': JSON.stringify({
      timestamp: new Date().toISOString(),
      summary: {
        total_requests: data.metrics.http_reqs.values.count,
        avg_latency_ms: data.metrics.http_req_duration.values.avg,
        p95_latency_ms: data.metrics.http_req_duration.values['p(95)'],
        p99_latency_ms: data.metrics.http_req_duration.values['p(99)'],
        error_rate: data.metrics.errors.values.rate,
      },
      by_scenario: scenarioResults,
      thresholds_met: Object.entries(data.metrics).every(
        ([, m]) => !m.thresholds || Object.values(m.thresholds).every((t) => t.ok)
      ),
    }),
  };
}
