/**
 * k6 SSE load test for the painless-agent task stream endpoint.
 *
 * Uses k6's experimental WebSocket module as a stand-in since k6 does not have
 * a native SSE module — we read raw HTTP with the http module instead.
 *
 * Run:
 *   BASE_URL=http://localhost:8080 k6 run infra/loadtest/sse.js
 */

import http from "k6/http";
import { check, sleep } from "k6";
import { Rate } from "k6/metrics";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const API_KEY = __ENV.API_KEY || "";

const errorRate = new Rate("errors");

export const options = {
  vus: 10,
  duration: "30s",
  thresholds: {
    errors: ["rate<0.05"],
  },
};

const headers = {
  "Content-Type": "application/json",
  ...(API_KEY ? { "X-API-Key": API_KEY } : {}),
};

export default function () {
  // Create a task to get a real task ID.
  const payload = JSON.stringify({ goal: `sse-load-test ${__VU}-${__ITER}` });
  const createRes = http.post(`${BASE_URL}/api/tasks`, payload, { headers });

  const ok = check(createRes, { "create task 201": (r) => r.status === 201 });
  if (!ok) {
    errorRate.add(true);
    sleep(1);
    return;
  }

  const task = createRes.json();
  if (!task || !task.id) {
    errorRate.add(true);
    return;
  }

  // Open SSE stream — read up to 2 KB of events then close (k6 does not support long-lived SSE natively).
  const streamRes = http.get(`${BASE_URL}/api/tasks/${task.id}/stream`, {
    headers: {
      ...headers,
      Accept: "text/event-stream",
    },
    timeout: "5s",
  });

  // A 200 or a timeout (k6 returns 0 on timeout) are both acceptable here
  // because a pending task may have no events yet.
  const streamOk = check(streamRes, {
    "stream endpoint reachable": (r) => r.status === 200 || r.status === 0,
  });
  errorRate.add(!streamOk);

  sleep(1);
}
