import http from 'k6/http';
import { check } from 'k6';

// Configurable via environment variables
export const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8080';
// Load test API key: UUID (from migration) : secret
// The migration 008 creates an API key with ID '9d78136e-308b-49fd-967f-e62b9b91f1d8' and secret 'load-test-secret'
export const API_KEY = __ENV.API_KEY || '9d78136e-308b-49fd-967f-e62b9b91f1d8:load-test-secret';

// Common headers
export const HEADERS = {
    'X-API-Key': API_KEY,
    'Content-Type': 'application/json',
    'X-Relay-Tags': 'type:datacenter,region:us',
};

// Helper to generate a random request payload
export function generatePayload() {
    return JSON.stringify({
        url: 'http://mock-target',
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
