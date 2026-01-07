/**
 * Strategy Manager
 * Manages trading strategy signals and real-time SSE updates
 */

import { CONFIG } from './config.js';
import { safeGetElement, formatStrategyName, getTimeAgo, parseTimestamp } from './utils.js';
import { fetchStrategySignals, fetchSignalHistory } from './api.js';
import { createStrategySignalSSE, closeSSE } from './sse-handler.js';

// State
let strategyEventSource = null;
let activeStrategyFilter = 'ALL';
let renderedSignalIds = new Set();

/**
 * Initialize strategy system
 */
export function initStrategySystem() {
    const tbody = safeGetElement('signals-table-body', 'StrategyInit');
    if (!tbody) {
        console.error('Critical element missing: signals-table-body not found in DOM');
        return;
    }

    setupStrategyTabs();

    // Fetch initial signals first, then connect SSE
    fetchInitialSignals().then(() => {
        connectStrategySSE();
    });

    // Poll for outcome updates
    setInterval(() => {
        const pendingBadges = document.querySelectorAll('.outcome-pending');
        if (pendingBadges.length > 0 && activeStrategyFilter !== 'HISTORY') {
            fetchInitialSignals();
        }
    }, 30000); // Every 30 seconds
}

/**
 * Fetch initial signals based on active filter
 */
async function fetchInitialSignals() {
    const tbody = safeGetElement('signals-table-body', 'FetchSignals');
    const placeholder = safeGetElement('signals-placeholder', 'FetchSignals');
    const loading = safeGetElement('signals-loading', 'FetchSignals');

    if (!tbody) return;

    if (placeholder) placeholder.style.display = 'none';
    if (loading) loading.style.display = 'flex';

    try {
        const data = await fetchStrategySignals(activeStrategyFilter, CONFIG.LOOKBACK_MINUTES);

        if (loading) loading.style.display = 'none';

        if (data.signals && data.signals.length > 0) {
            // Sort by timestamp descending (newest first)
            const signals = data.signals.sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));

            signals.forEach(signal => {
                renderSignalRow(signal, true); // true = initial load
            });
        } else {
            if (placeholder && tbody.children.length === 0) {
                placeholder.style.display = 'flex';
            }
        }
    } catch (err) {
        console.error("Failed to fetch initial signals:", err);
        if (loading) loading.style.display = 'none';
        if (placeholder) placeholder.style.display = 'flex';
    }
}

/**
 * Setup strategy tab switching
 */
function setupStrategyTabs() {
    const tabs = document.querySelectorAll('.strategy-tab');

    tabs.forEach(tab => {
        tab.addEventListener('click', () => {
            tabs.forEach(t => t.classList.remove('active'));
            tab.classList.add('active');

            activeStrategyFilter = tab.dataset.strategy;

            const tbody = safeGetElement('signals-table-body', 'TabSwitch');
            if (tbody) {
                tbody.innerHTML = '';
            }
            renderedSignalIds.clear();

            if (activeStrategyFilter === 'HISTORY') {
                if (strategyEventSource) closeSSE(strategyEventSource);
                fetchHistorySignals();
            } else {
                fetchInitialSignals().then(() => {
                    connectStrategySSE();
                });
            }
        });
    });
}

/**
 * Connect to strategy signals SSE
 */
function connectStrategySSE() {
    if (strategyEventSource) {
        closeSSE(strategyEventSource);
    }

    const statusEl = safeGetElement('strategy-connection-status', 'SSE');
    const indicatorEl = safeGetElement('strategy-live-indicator', 'SSE');

    if (!statusEl || !indicatorEl) return;

    statusEl.textContent = 'Connecting...';
    indicatorEl.style.backgroundColor = '#FFD700';
    indicatorEl.style.animation = 'pulse 2s infinite';

    strategyEventSource = createStrategySignalSSE(activeStrategyFilter, {
        onConnected: () => {
            statusEl.textContent = 'Live';
            indicatorEl.style.backgroundColor = '#0ECB81';
            indicatorEl.style.animation = 'none';

            const placeholder = safeGetElement('signals-placeholder', 'SSE');
            if (placeholder) placeholder.style.display = 'none';
        },
        onSignal: (signal) => {
            renderSignalRow(signal);
        },
        onError: () => {
            statusEl.textContent = 'Reconnecting';
            indicatorEl.style.backgroundColor = '#F6465D';
            indicatorEl.style.animation = 'pulse 1s infinite';
        }
    });
}

/**
 * Fetch signal history
 */
async function fetchHistorySignals() {
    const symbol = document.getElementById('symbol-filter-input')?.value || '';
    const loading = safeGetElement('signals-loading', 'FetchHistory');
    const tbody = safeGetElement('signals-table-body', 'FetchHistory');
    const placeholder = safeGetElement('signals-placeholder', 'FetchHistory');

    if (!tbody) return;

    if (loading) loading.style.display = 'flex';
    if (placeholder) placeholder.style.display = 'none';
    tbody.innerHTML = '';
    renderedSignalIds.clear();

    try {
        const data = await fetchSignalHistory(symbol, 50);

        if (loading) loading.style.display = 'none';

        if (!data.signals || !Array.isArray(data.signals) || data.signals.length === 0) {
            if (placeholder) placeholder.style.display = 'flex';
            return;
        }

        data.signals.forEach(signal => {
            renderSignalRow(signal, true);
        });
    } catch (err) {
        console.error("Failed to fetch history:", err);
        if (loading) loading.style.display = 'none';
        if (placeholder) placeholder.style.display = 'flex';
    }
}

/**
 * Render a single signal row
 * @param {Object} signal - Signal data
 * @param {boolean} isInitialLoad - Whether this is initial load (append to end)
 */
function renderSignalRow(signal, isInitialLoad = false) {
    const tbody = safeGetElement('signals-table-body', 'Render');
    const placeholder = safeGetElement('signals-placeholder', 'Render');

    if (!tbody) return;

    const signalId = `${signal.stock_symbol}-${signal.strategy}-${signal.timestamp}`;
    if (renderedSignalIds.has(signalId)) return;

    if (placeholder) placeholder.style.display = 'none';

    // Decision badge
    let badgeClass = 'badge';
    let decisionIcon = '';
    if (signal.decision === 'BUY') {
        badgeClass = 'badge buy';
        decisionIcon = '📈';
    } else if (signal.decision === 'SELL') {
        badgeClass = 'badge sell';
        decisionIcon = '📉';
    } else if (signal.decision === 'WAIT') {
        badgeClass = 'badge unknown';
        decisionIcon = '⏸️';
    }

    // Format data
    const priceValue = signal.price ?? signal.trigger_price ?? 0;
    const price = new Intl.NumberFormat('id-ID').format(priceValue);

    const changeValue = signal.change ?? signal.price_change_pct ?? 0;
    const change = changeValue.toFixed(2);
    const changeSign = changeValue >= 0 ? '+' : '';
    const changeClass = changeValue >= 0 ? 'diff-positive' : 'diff-negative';
    const confidence = Math.round((signal.confidence || 0) * 100);

    // Confidence display
    const { confidenceClass, confidenceIcon, confidenceLabel } = getConfidenceInfo(confidence);

    // Time formatting
    const { timeAgo, fullTime } = getTimeInfo(signal);

    // Z-Score info
    const priceZScore = signal.price_z_score || 0;
    const volumeZScore = signal.volume_z_score || 0;
    const enhancedReason = signal.reason || '-';
    const zScoreInfo = `Price Z: ${priceZScore.toFixed(2)} | Vol Z: ${volumeZScore.toFixed(2)}`;

    // Create row
    const row = document.createElement('tr');
    row.innerHTML = `
        <td data-label="Waktu" class="col-time" title="${fullTime}">${timeAgo}</td>
        <td data-label="Saham" class="col-symbol">
            <strong class="clickable-symbol" onclick="if(window.openCandleModal) window.openCandleModal('${signal.stock_symbol}')">${signal.stock_symbol}</strong>
        </td>
        <td data-label="Strategi" title="${signal.strategy.replace(/_/g, ' ')}">${formatStrategyName(signal.strategy)}</td>
        <td data-label="Aksi"><span class="${badgeClass}">${decisionIcon} ${signal.decision}</span></td>
        <td data-label="Harga" class="col-price">Rp ${price}</td>
        <td data-label="Perubahan" class="text-right">
            <span class="${changeClass}">${changeSign}${change}%</span>
        </td>
        <td data-label="Keyakinan" class="text-right">
            <span class="${confidenceClass}" title="${confidenceLabel} (${confidence}%)">${confidenceIcon} ${confidence}%</span>
        </td>
        <td data-label="Hasil" class="text-center">
            ${renderOutcome(signal)}
        </td>
        <td data-label="Alasan" class="reason-cell" title="${zScoreInfo}">
            ${enhancedReason}
            ${priceZScore > 0 || volumeZScore > 0 ? `<div style="font-size:0.7em; color:#888; margin-top:4px;">${zScoreInfo}</div>` : ''}
        </td>
    `;

    // Animation
    row.style.opacity = '0';
    row.style.transform = 'translateY(-10px)';

    if (isInitialLoad) {
        tbody.appendChild(row);
    } else {
        if (tbody.firstChild) {
            tbody.insertBefore(row, tbody.firstChild);
        } else {
            tbody.appendChild(row);
        }
    }

    setTimeout(() => {
        row.style.transition = `all ${CONFIG.TRANSITION_DURATION}ms ease`;
        row.style.opacity = '1';
        row.style.transform = 'translateY(0)';
    }, CONFIG.ANIMATION_DELAY);

    renderedSignalIds.add(signalId);

    // Limit rows
    if (tbody.children.length > CONFIG.MAX_VISIBLE_SIGNALS) {
        tbody.removeChild(tbody.lastChild);
    }
}

/**
 * Get confidence display info
 * @param {number} confidence - Confidence percentage
 * @returns {Object} Confidence display info
 */
function getConfidenceInfo(confidence) {
    let confidenceClass = 'confidence-low';
    let confidenceIcon = '⚪';
    let confidenceLabel = 'Low';

    if (confidence >= 80) {
        confidenceClass = 'confidence-extreme';
        confidenceIcon = '🔴';
        confidenceLabel = 'Extreme';
    } else if (confidence >= 70) {
        confidenceClass = 'confidence-high';
        confidenceIcon = '🟠';
        confidenceLabel = 'High';
    } else if (confidence >= 50) {
        confidenceClass = 'confidence-medium';
        confidenceIcon = '🟡';
        confidenceLabel = 'Medium';
    }

    return { confidenceClass, confidenceIcon, confidenceLabel };
}

/**
 * Get time display info
 * @param {Object} signal - Signal object
 * @returns {Object} Time display info
 */
function getTimeInfo(signal) {
    let timeAgo = 'Baru saja';
    let fullTime = '-';

    const timestampValue = signal.timestamp || signal.generated_at || signal.detected_at || signal.created_at;

    if (timestampValue) {
        const date = parseTimestamp(timestampValue);
        if (date) {
            timeAgo = getTimeAgo(date);
            fullTime = date.toLocaleString('id-ID', {
                year: 'numeric',
                month: 'short',
                day: '2-digit',
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit'
            });
        } else {
            timeAgo = 'Waktu tidak valid';
            fullTime = 'Timestamp: ' + timestampValue;
        }
    } else {
        timeAgo = 'Waktu tidak tersedia';
        fullTime = 'Tidak ada data waktu';
    }

    return { timeAgo, fullTime };
}

/**
 * Render outcome badge
 * @param {Object} signal - Signal object
 * @returns {string} HTML string
 */
function renderOutcome(signal) {
    if (!signal.outcome) {
        return `<span class="outcome-badge outcome-pending">PENDING</span>`;
    }

    const profit = signal.profit_loss_pct || 0;
    let outcomeClass = 'outcome-loss';

    if (signal.outcome === 'WIN') {
        outcomeClass = 'outcome-win';
    } else if (signal.outcome === 'OPEN') {
        outcomeClass = 'badge'; // Use default blue/neutral badge style
        return `<span class="badge" style="background: var(--accent-blue); color: white;">OPEN (${profit >= 0 ? '+' : ''}${profit.toFixed(1)}%)</span>`;
    } else if (signal.outcome === 'SKIPPED') {
        return `<span class="badge" style="background: var(--accent-gold); color: white;">SKIPPED</span>`;
    } else if (signal.outcome === 'BREAKEVEN') {
        return `<span class="badge" style="background: var(--text-secondary); color: white;">BREAKEVEN</span>`;
    }

    const sign = profit >= 0 ? '+' : '';
    return `<span class="outcome-badge ${outcomeClass}">${signal.outcome} (${sign}${profit.toFixed(1)}%)</span>`;
}
