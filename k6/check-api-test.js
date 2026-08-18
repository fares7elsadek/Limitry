import http from "k6/http";
import { check, sleep } from "k6";
import { Rate, Counter, Trend } from "k6/metrics";

// Tell k6 that 429 is an expected response — it's the whole point of a
// rate limiter, so it must NOT count toward http_req_failed.
http.setResponseCallback(http.expectedStatuses(200, 429));

// ─── Custom Metrics ───────────────────────────────────────────────
const rateLimited = new Rate("rate_limited");
const rateLimitedCount = new Counter("rate_limited_count");
const allowedCount = new Counter("allowed_count");
const checkLatency = new Trend("check_latency", true);

// ─── Configuration ───────────────────────────────────────────────
const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";

// ─── Thresholds ──────────────────────────────────────────────────
// These determine whether the test passes or fails.
// Adjust based on your SLOs.
export const options = {
  thresholds: {
    http_req_duration: ["p(95)<500", "p(99)<1000"],
    http_req_failed: ["rate<0.10"],
    check_latency: ["p(95)<500"],
  },

  scenarios: {
    // Smoke test — basic sanity check
    smoke: {
      executor: "constant-vus",
      vus: 1,
      duration: "10s",
      exec: "checkLogin",
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
      exec: "checkMixed",
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
      exec: "checkLogin",
      startTime: "2m",
      tags: { scenario: "spike" },
    },
  },
};

// ─── Helpers ─────────────────────────────────────────────────────
const headers = { "Content-Type": "application/json" };

function postCheck(clientId, route) {
  const payload = JSON.stringify({
    client_id: clientId,
    route: route,
  });

  const res = http.post(`${BASE_URL}/check`, payload, { headers });

  checkLatency.add(res.timings.duration);

  const isRateLimited = res.status === 429;
  rateLimited.add(isRateLimited);

  if (isRateLimited) {
    rateLimitedCount.add(1);
  } else {
    allowedCount.add(1);
  }

  // Parse response body
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (_) {
    body = {};
  }

  check(res, {
    "status is 200 or 429": (r) => r.status === 200 || r.status === 429,
    "response has allowed field": () =>
      body.allowed !== undefined,
    "response has retry_after_seconds field": () =>
      body.retry_after_seconds !== undefined,
    "allowed field matches status": () =>
      (body.allowed === true && res.status === 200) ||
      (body.allowed === false && res.status === 429),
  });

  if (isRateLimited) {
    check(res, {
      "429 has retry_after_seconds >= 0": () =>
        body.retry_after_seconds >= 0,
    });
  }

  return res;
}

// ─── Scenarios ───────────────────────────────────────────────────

// Test /api/login route (sliding window, limit=5/60s)
// Uses unique client IDs per VU to isolate rate-limit buckets
export function checkLogin() {
  const clientId = `k6-login-vu${__VU}-${__ITER}`;
  postCheck(clientId, "/api/login");
  sleep(0.1);
}

// Mixed traffic across both routes
export function checkMixed() {
  const clientId = `k6-vu${__VU}`;

  // Hit /api/login — low limit, should trigger 429s quickly
  postCheck(clientId, "/api/login");
  sleep(0.05);

  // Hit /api/search — high limit, should mostly pass
  postCheck(clientId, "/api/search");
  sleep(0.05);
}

// ─── Lifecycle ───────────────────────────────────────────────────
export function setup() {
  // Verify Limitry is reachable before running tests
  const healthCheck = http.post(
    `${BASE_URL}/check`,
    JSON.stringify({ client_id: "k6-health", route: "/api/login" }),
    { headers }
  );

  const isUp = healthCheck.status === 200 || healthCheck.status === 429;

  if (!isUp) {
    throw new Error(
      `Limitry is not reachable at ${BASE_URL}. ` +
        `Got status ${healthCheck.status}. ` +
        `Make sure the service is running in check mode.`
    );
  }

  console.log(`✓ Limitry is reachable at ${BASE_URL}`);
  console.log("Starting k6 load test for Check API mode...");

  return { baseUrl: BASE_URL };
}

export function teardown(data) {
  console.log(`✓ k6 load test complete against ${data.baseUrl}`);
}
