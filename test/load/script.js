import http from 'k6/http';
import { check } from 'k6';

export const BASE_URL = (typeof __ENV !== 'undefined' && __ENV.BASE_URL) ? __ENV.BASE_URL : 'http://localhost:8080';
export const API_TOKEN = __ENV.API_TOKEN || 'load-test-token-67890';

export const HEADERS = {
  'Authorization': `Bearer ${API_TOKEN}`,
  'Content-Type': 'application/json',
  'X-Relay-Tags': 'type:datacenter,region:us',
};

export function generatePayload() {
  return JSON.stringify({
    url: 'http://mock-target:80',
    method: 'GET',
    headers: {
      'User-Agent': 'k6-load-test',
    },
  });
}

export function checkResponse(res) {
  return check(res, {
    'is status 200': (r) => r?.status === 200,
    'has valid response body': (r) => r?.body?.length > 0,
  });
}

export const options = {
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500'],
  },
  stages: [
    { duration: '30s', target: 50 },
    { duration: '1m', target: 50 },
    { duration: '30s', target: 0 },
  ],
};

export default function () {
  const payload = generatePayload();
  const res = http.post(`${BASE_URL}/v1/request`, payload, { headers: HEADERS });
  checkResponse(res);
}
