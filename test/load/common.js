import http from 'k6/http';
import { check } from 'k6';

// Configurable via environment variables
export const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8080';
// Load test API key token (from seed.sql)
// Token: load-test-token-67890
export const API_TOKEN = __ENV.API_TOKEN || 'load-test-token-67890';

// Common headers
export const HEADERS = {
    'Authorization': `Bearer ${API_TOKEN}`,
    'Content-Type': 'application/json',
    'X-Relay-Tags': 'type:datacenter,region:us',
};

// Helper to generate a random request payload
export function generatePayload() {
    return JSON.stringify({
        url: 'http://mock-target:80',  // Docker network DNS name - endpoints can reach this
        method: 'GET',
        headers: {
            'User-Agent': 'k6-load-test',
        },
    });
}

// Common checks for successful proxy requests
export function checkResponse(res) {
    return check(res, {
        'is status 200': (r) => r.status === 200,
        'has valid response body': (r) => r.body.length > 0,
    });
}

export const options = {
    thresholds: {
        http_req_failed: ['rate<0.01'], // http errors should be less than 1%
        http_req_duration: ['p(95)<500'], // 95% of requests should be below 500ms
    },
};
