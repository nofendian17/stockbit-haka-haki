/**
 * Strategy Manager
 * Manages trading strategy signals and real-time SSE updates
 */

import { CONFIG } from './config.js';
import { safeGetElement, formatStrategyName, getTimeAgo, parseTimestamp, setupTableInfiniteScroll, renderWhaleAlignmentBadge } from './utils.js?v=2';
import { fetchStrategySignals, fetchSignalHistory } from './api.js';
import { createStrategySignalSSE, closeSSE } from './sse-handler.js';

// State
let strategyEventSource = null;
let activeStrategyFilter = 'ALL';
let renderedSignalIds = new Set();
let historyState = {
    offset: 0,
    hasMore: true,
    isLoading: false
};
let isHistoryScrollSetup = false;

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
                fetchHistorySignals(true);
                setupHistoryInfiniteScroll();
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
    indicatorEl.style.backgroundColor = '#FFD700'; // Keep inline for dynamic color state, or map to classes if possible, but simplest to leave for status
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
/**
 * Fetch signal history
 * @param {boolean} reset - Whether to reset the list
 */
async function fetchHistorySignals(reset = false) {
    if (historyState.isLoading) return;
    if (!reset && !historyState.hasMore) return;

    const symbol = document.getElementById('symbol-filter-input')?.value || '';
    const loading = safeGetElement('signals-loading', 'FetchHistory');
    const tbody = safeGetElement('signals-table-body', 'FetchHistory');
    const placeholder = safeGetElement('signals-placeholder', 'FetchHistory');
    const loadingMore = safeGetElement('signals-loading-more'); // Need to ensure this element exists or handle gracefully

    if (!tbody) return;

    historyState.isLoading = true;

    if (reset) {
        historyState.offset = 0;
        historyState.hasMore = true;
        tbody.innerHTML = '';
        renderedSignalIds.clear();
        if (loading) loading.style.display = 'flex';
        if (placeholder) placeholder.style.display = 'none';
    } else {
        // Show loading more indicator if exists, or reuse main loading
        if (loadingMore) loadingMore.style.display = 'flex';
    }

    try {
        const offset = reset ? 0 : historyState.offset;
        const limit = 50;
        const data = await fetchSignalHistory(symbol, limit, offset);

        if (reset && loading) loading.style.display = 'none';
        if (loadingMore) loadingMore.style.display = 'none';

        const signals = data.signals || [];
        const count = data.count || signals.length; // Use count from backend if available

        if (signals.length === 0) {
            historyState.hasMore = false;
            if (reset && placeholder) placeholder.style.display = 'flex';
            return;
        }

        // Update state
        historyState.offset += signals.length;
        historyState.hasMore = signals.length >= limit; // Simple check, or use data.has_more if backend provides

        signals.forEach(signal => {
            renderSignalRow(signal, true); // Treat history items as initial load (append)
        });

    } catch (err) {
        console.error("Failed to fetch history:", err);
        historyState.isLoading = false;
        if (reset && loading) loading.style.display = 'none';
        if (loadingMore) loadingMore.style.display = 'none';
        if (reset && placeholder) placeholder.style.display = 'flex';
    } finally {
        historyState.isLoading = false;
    }
}

/**
 * Setup infinite scroll for history table
 */
function setupHistoryInfiniteScroll() {
    if (isHistoryScrollSetup) return;

    setupTableInfiniteScroll({
        tableBodyId: 'signals-table-body',
        fetchFunction: () => fetchHistorySignals(false),
        getHasMore: () => historyState.hasMore,
        getIsLoading: () => historyState.isLoading,
        noMoreDataId: 'signals-no-more-data' // Ensure this ID exists in HTML
    });

    isHistoryScrollSetup = true;
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
    let badgeClass = 'bg-gray-700 text-gray-300';
    let decisionIcon = '';
    if (signal.decision === 'BUY') {
        badgeClass = 'bg-accentSuccess/20 text-accentSuccess border border-accentSuccess/20';
        decisionIcon = '📈';
    } else if (signal.decision === 'SELL') {
        badgeClass = 'bg-accentDanger/20 text-accentDanger border border-accentDanger/20';
        decisionIcon = '📉';
    } else if (signal.decision === 'WAIT') {
        badgeClass = 'bg-gray-700 text-gray-300';
        decisionIcon = '⏸️';
    }

    // Format data
    const priceValue = signal.price ?? signal.trigger_price ?? 0;
    const price = new Intl.NumberFormat('id-ID').format(priceValue);

    const changeValue = signal.change ?? signal.price_change_pct ?? 0;
    const change = changeValue.toFixed(2);
    const changeSign = changeValue >= 0 ? '+' : '';
    const changeClass = changeValue >= 0 ? 'text-accentSuccess font-bold' : 'text-accentDanger font-bold';
    const confidence = Math.round((signal.confidence || 0) * 100);

    // Confidence display
    const { confidenceClass, confidenceIcon, confidenceLabel } = getConfidenceInfo(confidence);

    // Time formatting
    const { timeAgo, fullTime } = getTimeInfo(signal);

    // Z-Score info
    const priceZScore = signal.price_z_score || 0;
    const volumeZScore = signal.volume_z_score || 0;
    const enhancedReason = signal.reason || '-';
    // const zScoreInfo = `Price Z: ${priceZScore.toFixed(2)} | Vol Z: ${volumeZScore.toFixed(2)}`;

    // NEW: Whale alignment badge
    const whaleBadge = renderWhaleAlignmentBadge(signal);

    // Create row
    const row = document.createElement('tr');
    row.className = 'border-b border-borderColor last:border-0 hover:bg-bgHover transition-colors';

    row.innerHTML = `
        <td data-label="Waktu" class="table-cell whitespace-nowrap text-textMuted text-xs" title="${fullTime}">${timeAgo}</td>
        <td data-label="Saham" class="table-cell font-bold">
            <strong class="cursor-pointer hover:text-accentInfo transition-colors" onclick="if(window.openCandleModal) window.openCandleModal('${signal.stock_symbol}')">${signal.stock_symbol}</strong>
            <div class="flex gap-1 mt-1">
                ${whaleBadge}
            </div>
        </td>
        <td data-label="Strategi" class="table-cell text-xs" title="${signal.strategy.replace(/_/g, ' ')}">${formatStrategyName(signal.strategy)}</td>
        <td data-label="Aksi" class="table-cell"><span class="px-2 py-0.5 rounded text-xs font-bold ${badgeClass}">${decisionIcon} ${signal.decision}</span></td>
        <td data-label="Harga" class="table-cell text-right font-medium text-sm">Rp ${price}</td>
        <td data-label="Perubahan" class="table-cell text-right text-sm">
            <span class="${changeClass}">${changeSign}${change}%</span>
        </td>
        <td data-label="Result" class="table-cell text-center">
            ${renderOutcome(signal)}
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
    let confidenceClass = 'text-textMuted';
    let confidenceIcon = '⚪';
    let confidenceLabel = 'Low';

    if (confidence >= 80) {
        confidenceClass = 'text-accentDanger font-bold';
        confidenceIcon = '🔴';
        confidenceLabel = 'Extreme';
    } else if (confidence >= 70) {
        confidenceClass = 'text-accentWarning font-bold';
        confidenceIcon = '🟠';
        confidenceLabel = 'High';
    } else if (confidence >= 50) {
        confidenceClass = 'text-yellow-400 font-bold';
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
        return `<span class="outcome-pending px-2 py-0.5 bg-bgSecondary text-textSecondary text-xs rounded border border-borderColor">PENDING</span>`;
    }

    const profit = signal.profit_loss_pct || 0;

    if (signal.outcome === 'WIN') {
        return `<span class="px-2 py-0.5 bg-accentSuccess/20 text-accentSuccess border border-accentSuccess/20 text-xs rounded font-bold">WIN (+${profit.toFixed(1)}%)</span>`;
    } else if (signal.outcome === 'OPEN') {
        return `<span class="px-2 py-0.5 bg-accentInfo/20 text-accentInfo border border-accentInfo/20 text-xs rounded font-bold">OPEN (${profit >= 0 ? '+' : ''}${profit.toFixed(1)}%)</span>`;
    } else if (signal.outcome === 'LOSS') {
        return `<span class="px-2 py-0.5 bg-accentDanger/20 text-accentDanger border border-accentDanger/20 text-xs rounded font-bold">LOSS (${profit.toFixed(1)}%)</span>`;
    } else if (signal.outcome === 'SKIPPED') {
        return `<span class="px-2 py-0.5 bg-yellow-900/40 text-yellow-400 border border-yellow-700/40 text-xs rounded font-bold">SKIPPED</span>`;
    } else if (signal.outcome === 'BREAKEVEN') {
        return `<span class="px-2 py-0.5 bg-gray-700 text-gray-300 text-xs rounded font-bold">BREAKEVEN</span>`;
    }

    return `<span class="text-xs text-textMuted">${signal.outcome}</span>`;
}
