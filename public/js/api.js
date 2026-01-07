/**
 * API Communication Layer
 * Handles all HTTP requests to the backend
 */

import { API_ENDPOINTS, CONFIG } from './config.js';

/**
 * Base fetch wrapper with error handling
 * @param {string} url - API endpoint URL
 * @param {Object} options - Fetch options
 * @returns {Promise<any>} Response data
 */
async function apiFetch(url, options = {}) {
    try {
        const response = await fetch(url, {
            headers: {
                'Content-Type': 'application/json',
                ...options.headers,
            },
            ...options,
        });

        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        // Handle 204 No Content
        if (response.status === 204) {
            return null;
        }

        const data = await response.json();
        return data;
    } catch (error) {
        console.error(`API Error [${url}]:`, error);
        throw error;
    }
}

/**
 * Fetch whale alerts with filters
 * @param {Object} filters - Filter parameters
 * @param {number} offset - Pagination offset
 * @returns {Promise<Object>} Alerts data
 */
export async function fetchAlerts(filters = {}, offset = 0) {
    const params = new URLSearchParams();

    if (filters.search) params.append('symbol', filters.search.toUpperCase());
    if (filters.action && filters.action !== 'ALL') params.append('action', filters.action);
    if (filters.amount && filters.amount > 0) params.append('min_value', filters.amount);
    if (filters.board && filters.board !== 'ALL') params.append('board', filters.board);

    params.append('limit', CONFIG.PAGE_SIZE);
    params.append('offset', offset);

    const url = `${API_ENDPOINTS.ALERTS}?${params.toString()}`;
    return apiFetch(url);
}

/**
 * Fetch global statistics
 * @returns {Promise<Object>} Stats data
 */
export async function fetchStats() {
    return apiFetch(API_ENDPOINTS.STATS);
}

/**
 * Fetch accumulation/distribution summary
 * @returns {Promise<Object>} Summary data
 */
export async function fetchAccumulationSummary() {
    return apiFetch(API_ENDPOINTS.ACCUMULATION_SUMMARY);
}



/**
 * Fetch analytics hub data (correlations, performance)
 * @returns {Promise<Object>} Analytics data
 */
export async function fetchAnalyticsHub() {
    try {
        // Fetch available analytics endpoints
        const [performance, orderFlow] = await Promise.all([
            fetchDailyPerformance().catch(() => null),
            fetchOrderFlow().catch(() => null)
        ]);

        return {
            performance: performance || {},
            orderFlow: orderFlow || {}
        };
    } catch (error) {
        console.error('Failed to fetch analytics hub:', error);
        return { performance: {}, orderFlow: {} };
    }
}

/**
 * Fetch order flow data
 * @returns {Promise<Object>} Order flow data
 */
export async function fetchOrderFlow() {
    return apiFetch(API_ENDPOINTS.ORDER_FLOW);
}

/**
 * Fetch running/open positions
 * @returns {Promise<Object>} Positions data
 */
export async function fetchRunningPositions() {
    return apiFetch(API_ENDPOINTS.POSITIONS_OPEN);
}

/**
 * Fetch strategy signals
 * @param {string} strategy - Strategy filter (ALL, VOLUME_BREAKOUT, etc.)
 * @param {number} lookback - Lookback minutes
 * @returns {Promise<Object>} Signals data
 */
export async function fetchStrategySignals(strategy = 'ALL', lookback = CONFIG.LOOKBACK_MINUTES) {
    const params = new URLSearchParams();
    params.append('lookback', lookback);

    if (strategy !== 'ALL') {
        params.append('strategy', strategy);
    }

    const url = `${API_ENDPOINTS.STRATEGIES_SIGNALS}?${params.toString()}`;
    return apiFetch(url);
}

/**
 * Fetch signal history
 * @param {string} symbol - Optional symbol filter
 * @param {number} limit - Number of records to fetch
 * @returns {Promise<Object>} Signal history data
 */
export async function fetchSignalHistory(symbol = '', limit = 50) {
    const params = new URLSearchParams();
    params.append('limit', limit);

    if (symbol) {
        params.append('symbol', symbol.trim().toUpperCase());
    }

    const url = `${API_ENDPOINTS.SIGNALS_HISTORY}?${params.toString()}`;
    return apiFetch(url);
}

/**
 * Fetch candle data for a symbol
 * @param {string} symbol - Stock symbol
 * @param {string} timeframe - Timeframe (5m, 15m, 1h, 1d)
 * @returns {Promise<Object>} Candle data
 */
export async function fetchCandles(symbol, timeframe = '5m') {
    const params = new URLSearchParams();
    params.append('symbol', symbol.toUpperCase());
    params.append('timeframe', timeframe);

    const url = `${API_ENDPOINTS.CANDLES}?${params.toString()}`;
    return apiFetch(url);
}

/**
 * Fetch whale alert followup data
 * @param {number} alertId - Alert ID
 * @returns {Promise<Object>} Followup data
 */
export async function fetchWhaleFollowup(alertId) {
    const url = `${API_ENDPOINTS.FOLLOWUP}/${alertId}/followup`;
    return apiFetch(url);
}

/**
 * Fetch recent whale followups
 * @returns {Promise<Object>} Recent followups data
 */
export async function fetchRecentFollowups() {
    return apiFetch(API_ENDPOINTS.RECENT_FOLLOWUPS);
}

/**
 * Fetch stock correlations
 * @param {string} symbol - Stock symbol
 * @returns {Promise<Object>} Correlation data
 */
export async function fetchCorrelations(symbol) {
    const params = new URLSearchParams();
    params.append('symbol', symbol.toUpperCase());

    const url = `${API_ENDPOINTS.CORRELATIONS}?${params.toString()}`;
    return apiFetch(url);
}

/**
 * Fetch daily performance metrics
 * @returns {Promise<Object>} Performance data
 */
export async function fetchDailyPerformance() {
    return apiFetch(API_ENDPOINTS.PERFORMANCE);
}

/**
 * Fetch market intelligence data
 * @returns {Promise<Object>} Market intelligence data
 */
export async function fetchMarketIntelligence() {
    try {
        // Only fetch endpoints that don't require specific symbol
        const patterns = await fetchDetectedPatterns().catch(() => null);

        return {
            patterns: patterns || []
        };
    } catch (error) {
        console.error('Failed to fetch market intelligence:', error);
        return { patterns: [] };
    }
}

/**
 * Fetch statistical baselines for a specific symbol
 * @param {string} symbol - Stock symbol (required)
 * @returns {Promise<Object>} Statistical baselines
 */
export async function fetchStatisticalBaselines(symbol) {
    if (!symbol) {
        throw new Error('Symbol parameter is required');
    }
    const params = new URLSearchParams();
    params.append('symbol', symbol.toUpperCase());
    return apiFetch(`${CONFIG.API_BASE}/baselines?${params.toString()}`);
}

/**
 * Fetch market regime for a specific symbol
 * @param {string} symbol - Stock symbol (required)
 * @returns {Promise<Object>} Market regime data
 */
export async function fetchMarketRegime(symbol) {
    if (!symbol) {
        throw new Error('Symbol parameter is required');
    }
    const params = new URLSearchParams();
    params.append('symbol', symbol.toUpperCase());
    return apiFetch(`${CONFIG.API_BASE}/regimes?${params.toString()}`);
}

/**
 * Fetch detected patterns
 * @returns {Promise<Object>} Detected patterns
 */
export async function fetchDetectedPatterns() {
    return apiFetch(`${CONFIG.API_BASE}/patterns`);
}

/**
 * Fetch profit/loss history
 * @param {Object} filters - Filter parameters (strategy, status, limit, symbol)
 * @returns {Promise<Object>} P&L history data
 */
export async function fetchProfitLossHistory(filters = {}) {
    const params = new URLSearchParams();

    if (filters.strategy && filters.strategy !== 'ALL') {
        params.append('strategy', filters.strategy);
    }
    if (filters.status) {
        params.append('status', filters.status);
    }
    if (filters.limit) {
        params.append('limit', filters.limit);
    }
    if (filters.symbol) {
        params.append('symbol', filters.symbol.toUpperCase());
    }

    const url = `${API_ENDPOINTS.POSITIONS_HISTORY}?${params.toString()}`;
    return apiFetch(url);
}

/**
 * Webhook Management Functions
 */

/**
 * Fetch all webhooks
 * @returns {Promise<Array>} List of webhooks
 */
export async function fetchWebhooks() {
    return apiFetch(API_ENDPOINTS.WEBHOOKS);
}

/**
 * Create a new webhook
 * @param {Object} webhook - Webhook data
 * @returns {Promise<Object>} Created webhook
 */
export async function createWebhook(webhook) {
    return apiFetch(API_ENDPOINTS.WEBHOOKS, {
        method: 'POST',
        body: JSON.stringify(webhook),
    });
}

/**
 * Update an existing webhook
 * @param {number} id - Webhook ID
 * @param {Object} webhook - Updated webhook data
 * @returns {Promise<Object>} Updated webhook
 */
export async function updateWebhook(id, webhook) {
    return apiFetch(`${API_ENDPOINTS.WEBHOOKS}/${id}`, {
        method: 'PUT',
        body: JSON.stringify(webhook),
    });
}

/**
 * Delete a webhook
 * @param {number} id - Webhook ID
 * @returns {Promise<void>}
 */
export async function deleteWebhook(id) {
    return apiFetch(`${API_ENDPOINTS.WEBHOOKS}/${id}`, {
        method: 'DELETE',
    });
}

/**
 * Pattern Analysis Functions (Non-streaming)
 */

/**
 * Fetch accumulation patterns
 * @param {number} hoursBack - Hours to look back
 * @param {number} minAlerts - Minimum alerts threshold
 * @returns {Promise<Object>} Accumulation patterns
 */
export async function fetchAccumulationPattern(hoursBack = 24, minAlerts = 3) {
    const params = new URLSearchParams();
    params.append('hours', hoursBack);
    params.append('min_alerts', minAlerts);
    return apiFetch(`${API_ENDPOINTS.PATTERN_ACCUMULATION}?${params.toString()}`);
}

/**
 * Fetch extreme anomalies
 * @param {number} minZScore - Minimum Z-score threshold
 * @param {number} hoursBack - Hours to look back
 * @returns {Promise<Object>} Extreme anomalies
 */
export async function fetchExtremeAnomalies(minZScore = 5.0, hoursBack = 48) {
    const params = new URLSearchParams();
    params.append('min_z', minZScore);
    params.append('hours', hoursBack);
    return apiFetch(`${API_ENDPOINTS.PATTERN_ANOMALIES}?${params.toString()}`);
}

/**
 * Fetch time-based statistics
 * @param {number} daysBack - Days to look back
 * @returns {Promise<Object>} Time-based statistics
 */
export async function fetchTimeBasedStats(daysBack = 7) {
    const params = new URLSearchParams();
    params.append('days', daysBack);
    return apiFetch(`${API_ENDPOINTS.PATTERN_TIMING}?${params.toString()}`);
}

/**
 * Signal Performance Functions
 */

/**
 * Fetch signal performance statistics
 * @param {string} strategy - Strategy filter (optional)
 * @param {string} symbol - Symbol filter (optional)
 * @returns {Promise<Object>} Performance statistics
 */
export async function fetchSignalPerformance(strategy = '', symbol = '') {
    const params = new URLSearchParams();
    if (strategy) params.append('strategy', strategy);
    if (symbol) params.append('symbol', symbol.toUpperCase());
    return apiFetch(`${API_ENDPOINTS.SIGNAL_PERFORMANCE}?${params.toString()}`);
}

/**
 * Fetch signal outcome by signal ID
 * @param {number} signalId - Signal ID
 * @returns {Promise<Object>} Signal outcome
 */
export async function fetchSignalOutcome(signalId) {
    return apiFetch(`${API_ENDPOINTS.SIGNAL_OUTCOME}/${signalId}/outcome`);
}
