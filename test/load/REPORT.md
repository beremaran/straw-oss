# Load Test Report

## 1. Baseline Metrics (Smoke Test)

**Profile:** 10 VUs, 1 minute

- **Req/s Matches:** ~118.6/s
- **Latency (p95):** 65.05ms
- **Latency (p90):** 58.42ms
- **Error Rate:** 0.02% (3/10675)
- **Status:** PASS (Error rate < 0.1%)

## 2. Load Test Results

**Profile:** 50 VUs, 2 minute

- **Req/s Achieved:** ~154/s
- **Latency (p95):** 389.02ms
- **Latency (p90):** 338.27ms
- **Error Rate:** 0.01% (4/21577)
- **Status:** PASS (Error rate < 0.1%)

## 3. Stress Test Results

**Profile:** 200 VUs, 1.5 minute (ramping)

- **Req/s Achieved:** ~183/s
- **Latency (p95):** 1.37s
- **Latency (p90):** 1.1s
- **Error Rate:** 0.00% (1/16477)
- **Status:** FAIL (Threshold crossed: p(95) > 1s as expected for stress)
- **Observation:** System maintained low error rate despite high latency.

## 4. Soak Test Results

**Profile:** 200 VUs, 3.5 minute (shortened)

- **Req/s Achieved:** ~160/s
- **Latency (p95):** 2.11s
- **Latency (p90):** 1.78s
- **Error Rate:** 0.00% (2/32984)
- **Status:** FAIL (Threshold crossed due to high load)
- **Observation:** No stability issues detected during short run.
