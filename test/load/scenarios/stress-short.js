import http from 'k6/http';
import { BASE_URL, HEADERS, generatePayload, checkResponse, options as commonOptions } from '../common.js';

export const options = {
    ...commonOptions,
    stages: [
        { duration: '15s', target: 50 },  // Ramp up
        { duration: '30s', target: 100 }, // Hold
        { duration: '30s', target: 200 }, // Higher load
        { duration: '15s', target: 0 },   // Ramp down
    ],
    thresholds: {
        http_req_failed: ['rate<0.05'], // Allow higher failure rate (5%) during stress
        http_req_duration: ['p(95)<1000'], // Allow higher latency
    },
};

export default function () {
    const payload = generatePayload();
    const res = http.post(`${BASE_URL}/v1/request`, payload, { headers: HEADERS });
    checkResponse(res);
}
