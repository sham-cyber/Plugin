import http from 'k6/http';
import { check } from 'k6';

const baseURL = (__ENV.FRAUDGUARD_BASE_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '');
const loopbackBase = /^http:\/\/(localhost|127\.0\.0\.1)(:\d+)?$/i.test(baseURL);
if (!loopbackBase && __ENV.ALLOW_NONLOCAL_LOAD_TESTS !== 'true') {
  throw new Error('Refusing non-local load target; set ALLOW_NONLOCAL_LOAD_TESTS=true only for an authorized test environment.');
}

const requestedVUs = Number(__ENV.K6_VUS || '5');
if (!Number.isInteger(requestedVUs) || requestedVUs < 1 || requestedVUs > 50) {
  throw new Error('K6_VUS must be an integer from 1 to 50.');
}

const duration = __ENV.K6_DURATION || '30s';
const durationMatch = duration.match(/^(\d+)(s|m)$/);
if (!durationMatch) {
  throw new Error('K6_DURATION must be a number of seconds or minutes, for example 30s or 2m.');
}
const durationSeconds = Number(durationMatch[1]) * (durationMatch[2] === 'm' ? 60 : 1);
if (!Number.isInteger(durationSeconds) || durationSeconds < 1 || durationSeconds > 300) {
  throw new Error('K6_DURATION must be between 1 second and 5 minutes.');
}

const testKeys = (__ENV.FRAUDGUARD_K6_TEST_KEYS || 'sk-bf-fraudguard-local-demo')
  .split(',')
  .map((key) => key.trim())
  .filter(Boolean);
if (testKeys.length < 1 || testKeys.length > 32) {
  throw new Error('Provide between 1 and 32 test virtual keys through FRAUDGUARD_K6_TEST_KEYS.');
}

export const options = {
  vus: requestedVUs,
  duration,
  thresholds: {
    http_req_failed: ['rate<0.01'],
    checks: ['rate>0.99'],
  },
};

export default function () {
  const key = testKeys[(__VU + __ITER) % testKeys.length];
  const response = http.post(
    `${baseURL}/v1/chat/completions`,
    JSON.stringify({
      model: __ENV.FRAUDGUARD_K6_MODEL || 'vllm-local/model-a',
      messages: [{ role: 'user', content: 'Synthetic local FraudGuard load-test request.' }],
    }),
    {
      headers: {
        'Content-Type': 'application/json',
        'x-bf-vk': key,
      },
      timeout: '10s',
    },
  );

  check(response, {
    'local Bifrost request returned HTTP 200': (result) => result.status === 200,
  });
}
