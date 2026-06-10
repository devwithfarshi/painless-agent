/**
 * k6 load test for the painless-agent HTTP API.
 *
 * Install k6: https://k6.io/docs/get-started/installation/
 *
 * Run:
 *   BASE_URL=http://localhost:8080 k6 run infra/loadtest/api.js
 *
 * Targets:
 *   - p95 latency < 500 ms for read endpoints
 *   - error rate < 1%
 *   - 50 VUs sustained for 30 s
 */

import http from "k6/http";
import { check, sleep } from "k6";
import { Rate, Trend } from "k6/metrics";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const API_KEY = __ENV.API_KEY || "";

const errorRate = new Rate("errors");
const taskCreateDuration = new Trend("task_create_duration", true);
const taskListDuration = new Trend("task_list_duration", true);
const healthDuration = new Trend("health_duration", true);

export const options = {
  stages: [
    { duration: "10s", target: 20 }, // ramp up
    { duration: "30s", target: 50 }, // sustained load
    { duration: "10s", target: 0 }, // ramp down
  ],
  thresholds: {
    errors: ["rate<0.01"], // < 1% error rate
    http_req_duration: ["p(95)<500"], // 95th percentile < 500 ms
    task_list_duration: ["p(95)<200"],
    health_duration: ["p(95)<100"],
  },
};

const headers = {
  "Content-Type": "application/json",
  ...(API_KEY ? { "X-API-Key": API_KEY } : {}),
};

export default function () {
  // 1. Health check
  {
    const start = Date.now();
    const res = http.get(`${BASE_URL}/api/health`, { headers });
    healthDuration.add(Date.now() - start);
    const ok = check(res, {
      "health 200": (r) => r.status === 200,
      "health ok": (r) => r.json("status") === "ok",
    });
    errorRate.add(!ok);
  }

  // 2. List tasks
  {
    const start = Date.now();
    const res = http.get(`${BASE_URL}/api/tasks`, { headers });
    taskListDuration.add(Date.now() - start);
    const ok = check(res, { "list tasks 200": (r) => r.status === 200 });
    errorRate.add(!ok);
  }

  // 3. Create a task (write path)
  {
    const payload = JSON.stringify({ goal: `load-test task ${__VU}-${__ITER}` });
    const start = Date.now();
    const res = http.post(`${BASE_URL}/api/tasks`, payload, { headers });
    taskCreateDuration.add(Date.now() - start);
    const ok = check(res, { "create task 201": (r) => r.status === 201 });
    errorRate.add(!ok);

    // If created, fetch the task by ID (round-trip).
    if (ok && res.status === 201) {
      const task = res.json();
      if (task && task.id) {
        const getRes = http.get(`${BASE_URL}/api/tasks/${task.id}`, { headers });
        check(getRes, { "get task 200": (r) => r.status === 200 });
      }
    }
  }

  sleep(0.5);
}

export function handleSummary(data) {
  return {
    stdout: JSON.stringify(data, null, 2),
    "loadtest-summary.json": JSON.stringify(data),
  };
}
