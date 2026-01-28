/**
 * Server-Sent Events (SSE) Handler
 * Manages real-time WebSocket/SSE connections for whale alerts
 */

import { CONFIG } from './config.js';

/**
 * Create and manage SSE connection for market events (Whales + Running Trades)
 * @param {Object} handlers - {onAlert, onTrade, onError}
 * @returns {EventSource} EventSource instance
 */
export function createWhaleAlertSSE(handlers) {
    // Support legacy signature (onAlert, onError)
    let onAlert, onTrade, onError;
    if (typeof handlers === 'function') {
        onAlert = arguments[0];
        onError = arguments[1];
    } else {
        ({ onAlert, onTrade, onError } = handlers);
    }

    const evtSource = new EventSource('/api/events');

    evtSource.onmessage = function (event) {
        try {
            const msg = JSON.parse(event.data);

            // Handle Whale Alerts
            if (msg.event === 'whale_alert' && msg.payload) {
                if (onAlert) onAlert(msg.payload);
            }

            // Handle Running Trades
            if (msg.event === 'trade' && msg.payload) {
                if (onTrade) onTrade(msg.payload);
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
 * Safely close an EventSource connection
 * @param {EventSource} eventSource - EventSource to close
 */
export function closeSSE(eventSource) {
    if (eventSource && eventSource.readyState !== EventSource.CLOSED) {
        eventSource.close();
    }
}
