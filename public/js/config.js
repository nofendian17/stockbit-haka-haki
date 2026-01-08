/**
 * Global Configuration
 * Centralized configuration for the Whale Radar application
 */

export const CONFIG = {
    // API Configuration
    API_BASE: '/api',

    // Pagination & Display
    PAGE_SIZE: 50,
    MAX_ALERTS_CACHE: 100,
    MAX_VISIBLE_SIGNALS: 100,

    // Polling Intervals (milliseconds)
    STATS_POLL_INTERVAL: 10000,      // 10 seconds
    ANALYTICS_POLL_INTERVAL: 30000,   // 30 seconds
    OUTCOMES_POLL_INTERVAL: 30000,    // 30 seconds

    // UI Timing
    SCROLL_THRESHOLD: 200,            // pixels from bottom to trigger load
    ANIMATION_DELAY: 10,              // ms before animation starts
    TRANSITION_DURATION: 300,         // ms for transitions
    SEARCH_DEBOUNCE_MS: 500,          // ms debounce for search input

    // Strategy Configuration
    LOOKBACK_MINUTES: 60,             // lookback period for signals
};

export const API_ENDPOINTS = {
    // Whale Alerts
    ALERTS: '/api/whales',
    STATS: '/api/whales/stats',
    EVENTS: '/api/events',

    // Strategies & Signals
    STRATEGIES_SIGNALS: '/api/strategies/signals',
    SIGNALS_HISTORY: '/api/signals/history',
    SIGNAL_PERFORMANCE: '/api/signals/performance',
    SIGNAL_OUTCOME: '/api/signals',  // Use /api/signals/{id}/outcome

    // Positions
    POSITIONS_OPEN: '/api/positions/open',
    POSITIONS_HISTORY: '/api/positions/history',

    // Analytics
    ACCUMULATION_SUMMARY: '/api/accumulation-summary',
    ORDER_FLOW: '/api/orderflow',
    CORRELATIONS: '/api/analytics/correlations',
    PERFORMANCE: '/api/analytics/performance/daily',

    // Strategy Optimization Analytics
    STRATEGY_EFFECTIVENESS: '/api/analytics/strategy-effectiveness',
    OPTIMAL_THRESHOLDS: '/api/analytics/optimal-thresholds',
    TIME_EFFECTIVENESS: '/api/analytics/time-effectiveness',
    EXPECTED_VALUES: '/api/analytics/expected-values',

    // Pattern Analysis (Non-streaming)
    PATTERN_ACCUMULATION: '/api/patterns/accumulation',
    PATTERN_ANOMALIES: '/api/patterns/anomalies',
    PATTERN_TIMING: '/api/patterns/timing',

    // Webhooks Configuration
    WEBHOOKS: '/api/config/webhooks',

    // Candles & Followup
    CANDLES: '/api/candles',
    FOLLOWUP: '/api/whales',  // Use /api/whales/{id}/followup
    RECENT_FOLLOWUPS: '/api/whales/followups',
};

export default CONFIG;

