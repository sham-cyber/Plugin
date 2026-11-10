export type LiveCostSummary = {
  period: string;
  estimatedCostUSD: number;
  requests: number;
  promptTokens: number;
  completionTokens: number;
  rateCardSource: string;
  rateCardAsOf: string;
};

export type LiveCostLoadResult =
  | { status: 'ready'; summary: LiveCostSummary }
  | { status: 'unauthorized' | 'unavailable' | 'invalid'; message: string };

const monthPattern = /^\d{4}-(0[1-9]|1[0-2])$/;

export const configuredLiveCostPath = (
  import.meta.env.VITE_FRAUDGUARD_COST_API_PATH as string | undefined
)?.trim() || '';

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isCount(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0;
}

export async function fetchLiveCostSummary(
  configuredPath: string,
  signal: AbortSignal,
): Promise<LiveCostLoadResult> {
  let url: URL;
  try {
    if (!configuredPath.startsWith('/api/')) {
      return { status: 'invalid', message: 'Live source must use a same-origin /api/ route.' };
    }
    url = new URL(configuredPath, window.location.origin);
    if (url.origin !== window.location.origin || !url.pathname.startsWith('/api/')) {
      return { status: 'invalid', message: 'Live source must use a same-origin /api/ route.' };
    }
  } catch {
    return { status: 'invalid', message: 'Live source path is invalid.' };
  }

  url.searchParams.set('month', new Date().toISOString().slice(0, 7));
  try {
    const response = await fetch(url, {
      method: 'GET',
      credentials: 'include',
      cache: 'no-store',
      headers: { Accept: 'application/json' },
      signal,
    });
    if (response.status === 401 || response.status === 403) {
      return { status: 'unauthorized', message: 'Sign-in or tenant authorization is required.' };
    }
    if (!response.ok) {
      return { status: 'unavailable', message: 'The configured live cost source is unavailable.' };
    }
    const payload: unknown = await response.json();
    if (!isRecord(payload) ||
      payload.schema_version !== 'asharatna.cost.v1' ||
      payload.authenticated !== true ||
      payload.tenant_scoped !== true ||
      payload.model_breakdown_complete !== true ||
      typeof payload.period !== 'string' ||
      !monthPattern.test(payload.period) ||
      typeof payload.estimated_cost_usd !== 'number' ||
      !Number.isFinite(payload.estimated_cost_usd) ||
      payload.estimated_cost_usd < 0 ||
      payload.estimated_cost_usd > 1_000_000_000 ||
      !isCount(payload.requests) ||
      !isCount(payload.prompt_tokens) ||
      !isCount(payload.completion_tokens) ||
      typeof payload.rate_card_source !== 'string' ||
      payload.rate_card_source.length > 120 ||
      typeof payload.rate_card_as_of !== 'string' ||
      payload.rate_card_as_of.length > 40) {
      return { status: 'invalid', message: 'The live source returned an incomplete or unsupported cost summary.' };
    }
    return {
      status: 'ready',
      summary: {
        period: payload.period,
        estimatedCostUSD: payload.estimated_cost_usd,
        requests: payload.requests,
        promptTokens: payload.prompt_tokens,
        completionTokens: payload.completion_tokens,
        rateCardSource: payload.rate_card_source,
        rateCardAsOf: payload.rate_card_as_of,
      },
    };
  } catch {
    if (signal.aborted) {
      throw new DOMException('Request aborted', 'AbortError');
    }
    return { status: 'unavailable', message: 'The configured live cost source is unavailable.' };
  }
}
