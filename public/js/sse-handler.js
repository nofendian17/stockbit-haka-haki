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
 * Create SSE connection for custom prompt analysis
 * @param {string} prompt - User's custom prompt
 * @param {Array<string>} symbols - Optional symbols to analyze
 * @param {number} hoursBack - Hours of data to include
 * @param {string} includeData - Types of data to include (comma-separated)
 * @param {Object} handlers - Event handlers {onChunk, onDone, onError}
 * @returns {Object} Object with abort controller for cancellation
 */
export function createCustomPromptSSE(prompt, symbols = [], hoursBack = 24, includeData = 'alerts,regimes', handlers = {}) {
    const { onChunk, onDone, onError } = handlers;

    const url = `/api/patterns/custom/stream`;
    const controller = new AbortController();

    const requestBody = {
        prompt: prompt,
        symbols: symbols,
        hours_back: hoursBack,
        include_data: includeData
    };

    fetch(url, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify(requestBody),
        signal: controller.signal
    })
    .then(response => {
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        function processStream() {
            reader.read().then(({ done, value }) => {
                if (done) {
                    if (onDone) onDone();
                    return;
                }

                buffer += decoder.decode(value, { stream: true });
                const lines = buffer.split('\n');
                buffer = lines.pop(); // Keep incomplete line in buffer

                for (const line of lines) {
                    if (line.startsWith('data: ')) {
                        const chunk = line.substring(6);
                        if (onChunk) onChunk(chunk);
                    } else if (line.startsWith('event: done')) {
                        if (onDone) onDone();
                        return;
                    } else if (line.startsWith('event: error')) {
                        if (onError) onError(new Error('Stream error'));
                        return;
                    }
                }

                processStream();
            }).catch(err => {
                if (err.name !== 'AbortError') {
                    console.error('Custom Prompt SSE Error:', err);
                    if (onError) onError(err);
                }
            });
        }

        processStream();
    })
    .catch(err => {
        if (err.name !== 'AbortError') {
            console.error('Custom Prompt Fetch Error:', err);
            if (onError) onError(err);
        }
    });

    // Return object with close method for consistency
    return {
        close: () => controller.abort(),
        readyState: controller.signal.aborted ? 2 : 1
    };
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
