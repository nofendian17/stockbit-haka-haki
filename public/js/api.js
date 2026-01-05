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
    return apiFetch(API_ENDPOINTS.ANALYTICS_HUB);
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
        params.append('symbol', symbol.toUpperCase());
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
    const url = `${API_ENDPOINTS.FOLLOWUP}/${alertId}`;
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
