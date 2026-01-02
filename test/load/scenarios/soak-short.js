import http from 'k6/http';
import { BASE_URL, HEADERS, generatePayload, checkResponse, options as commonOptions } from '../common.js';

export const options = {
    ...commonOptions,
    stages: [
        { duration: '30s', target: 200 }, // Ramp up to 200
        { duration: '2m', target: 200 },  // Hold for 2 min
        { duration: '30s', target: 0 },   // Ramp down
    ],
};

export default function () {
    const payload = generatePayload();
    const res = http.post(`${BASE_URL}/v1/request`, payload, { headers: HEADERS });
    checkResponse(res);
}
