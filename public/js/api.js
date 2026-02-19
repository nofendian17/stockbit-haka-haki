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
 * @param {number} limit - Number of records per type (accumulation/distribution)
 * @param {number} offset - Pagination offset
 * @returns {Promise<Object>} Summary data
 */
export async function fetchAccumulationSummary(limit = 50, offset = 0) {
    const params = new URLSearchParams();
    params.append('limit', limit);
    params.append('offset', offset);

    const url = `${API_ENDPOINTS.ACCUMULATION_SUMMARY}?${params.toString()}`;
    return apiFetch(url);
}



/**
 * Fetch analytics hub data (correlations, performance)
 * @returns {Promise<Object>} Analytics data
 */
export async function fetchAnalyticsHub() {
    try {
        // Fetch available analytics endpoints
        const performance = await fetchDailyPerformance().catch(() => null);

        return {
            performance: performance || {}
        };
    } catch (error) {
        console.error('Failed to fetch analytics hub:', error);
        return { performance: {} };
    }
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
 * @param {number} offset - Pagination offset
 * @returns {Promise<Object>} Signal history data
 */
export async function fetchSignalHistory(symbol = '', limit = 50, offset = 0) {
    const params = new URLSearchParams();
    params.append('limit', limit);
    params.append('offset', offset);

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
 * @param {number} limit - Number of records to fetch
 * @param {number} offset - Pagination offset
 * @returns {Promise<Object>} Performance data
 */
export async function fetchDailyPerformance(limit = 50, offset = 0) {
    const params = new URLSearchParams();
    params.append('limit', limit);
    params.append('offset', offset);

    const url = `${API_ENDPOINTS.PERFORMANCE}?${params.toString()}`;
    return apiFetch(url);
}



/**
 * Fetch profit/loss history
 * @param {Object} filters - Filter parameters (strategy, status, limit, symbol, offset)
 * @returns {Promise<Object>} P&L history data
 */
export async function fetchProfitLossHistory(filters = {}) {
    const params = new URLSearchParams();

    if (filters.strategy && filters.strategy !== 'ALL') {
        params.append('strategy', filters.strategy);
    }
    if (filters.status && filters.status !== 'ALL') {
        params.append('status', filters.status);
    }
    if (filters.limit) {
        params.append('limit', filters.limit);
    }
    if (filters.offset !== undefined) {
        params.append('offset', filters.offset);
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

/**
 * Strategy Optimization Analytics Functions
 */

/**
 * Fetch strategy effectiveness
 * @param {number} daysBack - Days to look back
 * @returns {Promise<Object>} Strategy effectiveness data
 */
export async function fetchStrategyEffectiveness(daysBack = 30) {
    const params = new URLSearchParams();
    params.append('days', daysBack);
    return apiFetch(`${API_ENDPOINTS.STRATEGY_EFFECTIVENESS}?${params.toString()}`);
}

/**
 * Fetch optimal confidence thresholds per strategy
 * @param {number} daysBack - Days to look back
 * @returns {Promise<Object>} Optimal thresholds data
 */
export async function fetchOptimalThresholds(daysBack = 30) {
    const params = new URLSearchParams();
    params.append('days', daysBack);
    return apiFetch(`${API_ENDPOINTS.OPTIMAL_THRESHOLDS}?${params.toString()}`);
}

/**
 * Fetch time-of-day effectiveness
 * @param {number} daysBack - Days to look back
 * @returns {Promise<Object>} Time effectiveness data
 */
export async function fetchTimeEffectiveness(daysBack = 30) {
    const params = new URLSearchParams();
    params.append('days', daysBack);
    return apiFetch(`${API_ENDPOINTS.TIME_EFFECTIVENESS}?${params.toString()}`);
}

/**
 * Fetch expected values for strategies
 * @param {number} daysBack - Days to look back
 * @returns {Promise<Object>} Expected values data
 */
export async function fetchExpectedValues(daysBack = 30) {
    const params = new URLSearchParams();
    params.append('days', daysBack);
    return apiFetch(`${API_ENDPOINTS.EXPECTED_VALUES}?${params.toString()}`);
}

/**
 * Fetch order flow data for a symbol
 * @param {string} symbol - Stock symbol (optional, empty for global)
 * @returns {Promise<Object>} Order flow data
 */
export async function fetchOrderFlow(symbol = '') {
    // Stub function - backend endpoint not implemented yet
    // Returns mock data structure expected by frontend
    return {
        symbol: symbol || 'GLOBAL',
        buy_pressure: 0,
        sell_pressure: 0,
        net_flow: 0,
        timestamp: new Date().toISOString()
    };
}

/**
 * Create SSE connection for AI symbol analysis
 * @param {string} symbol - Stock symbol to analyze
 * @param {Object} handlers - Event handlers {onMessage, onError, onDone}
 * @returns {EventSource} EventSource instance
 */
export function createAISymbolAnalysisStream(symbol, handlers = {}) {
    const { onMessage, onError, onDone } = handlers;
    
    const params = new URLSearchParams();
    params.append('symbol', symbol.toUpperCase());
    
    const url = `${API_ENDPOINTS.AI_SYMBOL_ANALYSIS}?${params.toString()}`;
    const evtSource = new EventSource(url);
    
    let analysisText = '';
    
    evtSource.onmessage = (event) => {
        if (event.data === '[DONE]') {
            evtSource.close();
            if (onDone) onDone(analysisText);
            return;
        }
        
        analysisText += event.data;
        if (onMessage) onMessage(event.data, analysisText);
    };
    
    evtSource.onerror = (err) => {
        console.error('AI Analysis SSE Error:', err);
        if (onError) onError(err);
        evtSource.close();
    };
    
    return evtSource;
}

/**
 * Send custom prompt to AI for analysis
 * @param {string} prompt - User's question/prompt
 * @param {Object} options - Options {symbols, hoursBack, includeData}
 * @param {Object} handlers - Event handlers {onMessage, onError, onDone}
 * @returns {EventSource} EventSource instance
 */
export function createAICustomPromptStream(prompt, options = {}, handlers = {}) {
    const { onMessage, onError, onDone } = handlers;
    
    const body = {
        prompt,
        symbols: options.symbols || [],
        hours_back: options.hoursBack || 24,
        include_data: options.includeData || 'alerts'
    };
    
    // For POST with SSE, we need to use fetch with ReadableStream
    const controller = new AbortController();
    
    fetch(API_ENDPOINTS.AI_CUSTOM_PROMPT, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify(body),
        signal: controller.signal
    }).then(response => {
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let analysisText = '';
        
        function readStream() {
            reader.read().then(({ done, value }) => {
                if (done) {
                    if (onDone) onDone(analysisText);
                    return;
                }
                
                const chunk = decoder.decode(value, { stream: true });
                const lines = chunk.split('\n');
                
                lines.forEach(line => {
                    if (line.startsWith('data: ')) {
                        const data = line.slice(6);
                        if (data === '[DONE]') {
                            if (onDone) onDone(analysisText);
                            reader.cancel();
                            return;
                        }
                        analysisText += data + '\n';
                        if (onMessage) onMessage(data, analysisText);
                    }
                });
                
                readStream();
            }).catch(err => {
                if (onError) onError(err);
            });
        }
        
        readStream();
    }).catch(err => {
        console.error('AI Custom Prompt Error:', err);
        if (onError) onError(err);
    });
    
    return controller;
}

