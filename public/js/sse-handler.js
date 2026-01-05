/**
 * Server-Sent Events (SSE) Handler
 * Manages real-time WebSocket/SSE connections for whale alerts
 */

import { CONFIG } from './config.js';

/**
 * Create and manage SSE connection for whale alerts
 * @param {Function} onAlert - Callback when new alert received
 * @param {Function} onError - Callback on error
 * @returns {EventSource} EventSource instance
 */
export function createWhaleAlertSSE(onAlert, onError) {
    const evtSource = new EventSource('/api/events');

    evtSource.onmessage = function (event) {
        try {
            const msg = JSON.parse(event.data);
            if (msg.event === 'whale_alert' && msg.payload) {
                onAlert(msg.payload);
            }
        } catch (e) {
            console.error("SSE Parse Error:", e);
            if (onError) onError(e);
        }
    };

    evtSource.onerror = function (err) {
        console.error("SSE Connection Error:", err);
        if (onError) onError(err);
    };

    return evtSource;
}

/**
 * Create SSE connection for strategy signals
 * @param {string} strategy - Strategy filter
 * @param {Object} handlers - Event handlers {onConnected, onSignal, onError}
 * @returns {EventSource} EventSource instance
 */
export function createStrategySignalSSE(strategy = 'ALL', handlers = {}) {
    const { onConnected, onSignal, onError } = handlers;

    let url = '/api/strategies/signals/stream';
    if (strategy !== 'ALL') {
        url += `?strategy=${strategy}`;
    }

    const evtSource = new EventSource(url);

    evtSource.addEventListener('connected', (e) => {
        console.log('Strategy SSE Connected');
        if (onConnected) onConnected(e);
    });

    evtSource.addEventListener('signal', (e) => {
        try {
            const signal = JSON.parse(e.data);
            if (onSignal) onSignal(signal);
        } catch (err) {
            console.error('Error parsing signal:', err);
            if (onError) onError(err);
        }
    });

    evtSource.addEventListener('error', (e) => {
        console.error('Strategy SSE Error');
        if (onError) onError(e);
    });

    return evtSource;
}

/**
 * Create SSE connection for pattern analysis streaming
 * @param {string} patternType - Pattern type (accumulation, anomalies, timing, symbol)
 * @param {string} symbol - Optional symbol for symbol-specific analysis
 * @param {Object} handlers - Event handlers {onChunk, onDone, onError}
 * @returns {EventSource} EventSource instance
 */
export function createPatternAnalysisSSE(patternType, symbol = '', handlers = {}) {
    const { onChunk, onDone, onError } = handlers;

    let url = `/api/patterns/${patternType}/stream`;
    if (patternType === 'symbol' && symbol) {
        url = `/api/patterns/symbol/stream?symbol=${symbol.toUpperCase()}&limit=20`;
    }

    const evtSource = new EventSource(url);

    evtSource.onmessage = (event) => {
        const chunk = event.data;
        if (onChunk) onChunk(chunk);
    };

    evtSource.addEventListener('done', () => {
        if (onDone) onDone();
        evtSource.close();
    });

    evtSource.onerror = (err) => {
        console.error('Pattern Analysis SSE Error:', err);
        if (onError) onError(err);
    };

    return evtSource;
}

/**
 * Safely close an EventSource connection
 * @param {EventSource} eventSource - EventSource to close
 */
export function closeSSE(eventSource) {
    if (eventSource && eventSource.readyState !== EventSource.CLOSED) {
        eventSource.close();
    }
}
