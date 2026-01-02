import http from 'k6/http';
import { BASE_URL, HEADERS, generatePayload, checkResponse, options as commonOptions } from '../common.js';

export const options = {
    ...commonOptions,
    stages: [
        { duration: '5m', target: 500 },  // Ramp up to 500
        { duration: '10m', target: 1000 }, // Ramp up to 1000
        { duration: '10m', target: 2000 }, // Ramp up to 2000
        { duration: '5m', target: 0 },    // Ramp down
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
