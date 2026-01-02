import http from 'k6/http';
import { BASE_URL, HEADERS, generatePayload, checkResponse, options as commonOptions } from '../common.js';

export const options = {
    ...commonOptions,
    stages: [
        { duration: '2m', target: 200 }, // Ramp up to 200
        { duration: '3h56m', target: 200 }, // Stay at 200 for ~4 hours
        { duration: '2m', target: 0 }, // Ramp down
    ],
};

export default function () {
    const payload = generatePayload();
    const res = http.post(`${BASE_URL}/v1/request`, payload, { headers: HEADERS });
    checkResponse(res);
}
