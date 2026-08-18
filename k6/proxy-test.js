import http from "k6/http";
import { check, sleep } from "k6";
import { Rate, Counter, Trend } from "k6/metrics";

// Tell k6 that non-5xx responses are expected — in proxy mode, 429 comes
// from Limitry (rate limited) and the backend may return any status code
// (200, 404, 405, etc). We're testing the rate limiter, not the backend.
http.setResponseCallback(
  http.expectedStatuses({ min: 100, max: 499 })
);

// ─── Custom Metrics ───────────────────────────────────────────────
const rateLimited = new Rate("rate_limited");
const rateLimitedCount = new Counter("rate_limited_count");
const allowedCount = new Counter("allowed_count");
const proxyLatency = new Trend("proxy_latency", true);

// ─── Configuration ───────────────────────────────────────────────
const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";

// ─── Thresholds ──────────────────────────────────────────────────
export const options = {
  thresholds: {
    http_req_duration: ["p(95)<500", "p(99)<1000"],
    http_req_failed: ["rate<0.10"],
    proxy_latency: ["p(95)<500"],
  },

  scenarios: {
    // Smoke test — basic sanity check
    smoke: {
      executor: "constant-vus",
      vus: 1,
      duration: "10s",
      exec: "proxyLogin",
      tags: { scenario: "smoke" },
    },

    // Load test — steady-state traffic
    load: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "30s", target: 50 },
        { duration: "1m", target: 50 },
        { duration: "10s", target: 0 },
      ],
      exec: "proxyMixed",
      startTime: "15s",
      tags: { scenario: "load" },
    },

    // Spike test — burst resilience
    spike: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "5s", target: 100 },
        { duration: "15s", target: 100 },
        { duration: "5s", target: 0 },
      ],
      exec: "proxyLogin",
      startTime: "2m",
      tags: { scenario: "spike" },
    },
  },
};

// ─── Helpers ─────────────────────────────────────────────────────

function proxyRequest(method, path, clientId) {
  const params = {
    headers: {
      "X-Client-Id": clientId,
    },
  };

  let res;
  if (method === "GET") {
    res = http.get(`${BASE_URL}${path}`, params);
  } else {
    res = http.post(`${BASE_URL}${path}`, null, params);
  }

  proxyLatency.add(res.timings.duration);

  const isRateLimited = res.status === 429;
  rateLimited.add(isRateLimited);

  if (isRateLimited) {
    rateLimitedCount.add(1);
  } else {
    allowedCount.add(1);
  }

  // In proxy mode, any non-429 response means the request went through
  // to the backend. The backend may return various codes, so we accept
  // anything that isn't a 5xx from Limitry itself.
  check(res, {
    "status is not 500 (limiter error)": (r) => r.status !== 500,
    "response received": (r) => r.body !== null,
  });

  if (isRateLimited) {
    check(res, {
      "429 has Retry-After header": (r) =>
        r.headers["Retry-After"] !== undefined &&
        r.headers["Retry-After"] !== "",
    });
  }

  return res;
}

// ─── Scenarios ───────────────────────────────────────────────────

// Test /api/login via proxy (sliding window, limit=5/60s)
export function proxyLogin() {
  const clientId = `k6-proxy-login-vu${__VU}-${__ITER}`;
  proxyRequest("POST", "/api/login", clientId);
  sleep(0.1);
}

// Mixed traffic across both routes via proxy
export function proxyMixed() {
  const clientId = `k6-proxy-vu${__VU}`;

  // Hit /api/login — low limit, should trigger 429s quickly
  proxyRequest("POST", "/api/login", clientId);
  sleep(0.05);

  // Hit /api/search — high limit, should mostly pass
  proxyRequest("GET", "/api/search", clientId);
  sleep(0.05);
}

// ─── Lifecycle ───────────────────────────────────────────────────
export function setup() {
  // Verify Limitry proxy is reachable
  const healthCheck = http.get(`${BASE_URL}/api/search`, {
    headers: { "X-Client-Id": "k6-health" },
  });

  // In proxy mode, we may get various responses depending on backend.
  // A 502/503 means the backend is unreachable; anything else means
  // Limitry is at least running.
  if (healthCheck.status === 502 || healthCheck.status === 503) {
    throw new Error(
      `Limitry proxy is running but the backend is unreachable at ${BASE_URL}. ` +
        `Got status ${healthCheck.status}. ` +
        `Make sure the backend service is running.`
    );
  }

  if (healthCheck.status === 0) {
    throw new Error(
      `Limitry is not reachable at ${BASE_URL}. ` +
        `Make sure the service is running in proxy mode.`
    );
  }

  console.log(`✓ Limitry proxy is reachable at ${BASE_URL}`);
  console.log("Starting k6 load test for Proxy mode...");

  return { baseUrl: BASE_URL };
}

export function teardown(data) {
  console.log(`✓ k6 load test complete against ${data.baseUrl}`);
}
