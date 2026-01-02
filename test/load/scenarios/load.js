import http from 'k6/http';
import { BASE_URL, HEADERS, generatePayload, checkResponse, options as commonOptions } from '../common.js';

export const options = {
    ...commonOptions,
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
